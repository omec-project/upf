# vim: syntax=py
# -*- mode: python -*-
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

from conf.parser import *
import inspect
import sys


def setup_globals():
    caller_frame = inspect.stack()[1][0]
    caller_globals = caller_frame.f_globals
    globals().update(caller_globals)


# ====================================================
#       Port Helpers
# ====================================================
dpdk_ports = {}


def link_modules(module, next_module, ogate=0, igate=0):
    # Connect module to next_module
    for _ in range(MAX_RETRIES):
        try:
            bess.connect_modules(module, next_module, ogate, igate)
        except bess.Error as e:
            if e.code == errno.EBUSY:
                break
            else:
                return
        except Exception as e:
            print(
                'Error connecting module {}:{}->{}:{}: {}. Retrying in {} secs...'
                .format(module, ogate, igate, next_module, e, SLEEP_S))
            time.sleep(SLEEP_S)
        else:
            break
    else:
        print('BESS module connection ({}:{}->{}:{}) failure.'.format(
            module, ogate, igate, next_module))
        return


def scan_dpdk_ports():
    idx = 0
    while True:
        try:
            intf = PMDPort(name="Port {}".format(idx), port_id=idx)
            if intf:
                # Need to declare mac so that we don't lose key during destroy_port
                mac = intf.mac_addr
                dpdk_ports[mac] = idx
                bess.destroy_port(intf.name)
        except bess.Error as e:
            if e.code == errno.ENODEV:
                break
            else:
                raise
        idx += 1
        # RTE_MAX_ETHPORTS is 32 and we need 2 for vdevs
        if idx == 30:
          break
    return True if dpdk_ports else False


class Port:
    def __init__(self, name, ext_addrs):
        self.name = name
        self.wid = None
        self.fpi = None
        self.fpo = None
        self.bpf = None
        self.rtr = None
        self.bpfgate = 0
        self.routes_table = None
        self.nat = None
        self.ext_addrs = ext_addrs

    def bpf_gate(self):
        if self.bpfgate < MAX_GATES - 2:
            self.bpfgate += 1
            return self.bpfgate
        else:
            raise Exception('Port {}: Out of BPF gates to allocate'.format(self.name))

    def detect_mode(self):
        mode = None
        try:
            peer_by_interface(self.name)
            mode = 'dpdk'
        except:
            mode = 'linux'
        return mode

    def init_fastpath(self, **kwargs):
        # Initialize PMDPort and RX/TX modules
        name = self.name
        fast = PMDPort(name="{}Fast".format(name), **kwargs)
        self.fpi = __bess_module__("{}FastPI".format(name), 'PortInc', port=fast.name)
        self.fpo = __bess_module__("{}FastPO".format(name), 'PortOut', port=fast.name)

        # Initialize BPF to classify incoming traffic to go to kernel and/or pipeline
        self.bpf = __bess_module__("{}FastBPF".format(name), 'BPF')
        self.bpf.clear()

        # Initialize route module
        self.rtr = __bess_module__("{}Routes".format(name), 'IPLookup')

        # Default route goes to Sink
        self.rtr.add(prefix='0.0.0.0', prefix_len=0, gate=MAX_GATES-1)
        s = Sink(name="{}bad_route".format(name))
        link_modules("{}Routes".format(name), "{}bad_route".format(name), MAX_GATES-1)

        # Attach fastpath to worker's root TC
        self.fpi.attach_task(wid=self.wid)

    def setup_port(self, idx, conf_mode, conf_workers):
        # Pick the worker handling this port
        self.wid = idx % conf_workers

        name = self.name
        wid = self.wid
        print('Setting up port {} on worker {}'.format(name,wid))

        # Detect the mode of this interface - DPDK/AF_XDP/AF_PACKET
        if conf_mode is None:
            conf_mode = self.detect_mode()

        if conf_mode in ['af_xdp', 'linux']:
            try:
                # Initialize kernel fastpath.
                # AF_XDP requires that num_rx_qs == num_tx_qs
                kwargs = {"vdev" : "net_af_xdp{},iface={},start_queue=0,queue_count={}"
                          .format(idx, name, conf_workers), "num_out_q": conf_workers, "num_inc_q": conf_workers}
                self.init_fastpath(**kwargs)
            except:
                if conf_mode == 'linux':
                    print('Failed to create AF_XDP socket for {}. Retrying with AF_PACKET socket...'.format(name))
                    conf_mode = 'af_packet'
                else:
                    print('Failed to create AF_XDP socket for {}. Exiting...'.format(name))
                    sys.exit()

        if conf_mode == 'af_packet':
            try:
                # Initialize kernel fastpath
                kwargs = {"vdev" : "net_af_packet{},iface={},qpairs={}".format(idx, name, conf_workers), "num_out_q": conf_workers}
                self.init_fastpath(**kwargs)
            except:
                print('Failed to create AF_PACKET socket for {}. Exiting...'.format(name))
                sys.exit()

        elif conf_mode == 'dpdk':
            kwargs = None
            pci = alias_by_interface(name)
            if pci is not None:
                kwargs = {"pci": pci, "num_out_q": conf_workers}
                try:
                    self.init_fastpath(**kwargs)
                except:
                    kwargs = None
                    print('Unable to initialize {} fastpath using alias {},\
                        falling back to scan'.format(name, pci))
            if kwargs is None:
                # Fallback to scanning ports
                # if port list is empty, scan for dpdk_ports first
                if not dpdk_ports and scan_dpdk_ports() == False:
                    print('Registered dpdk ports do not exist.')
                    sys.exit()
                # Initialize DPDK fastpath
                fidx = dpdk_ports.get(mac_by_interface(name))
                if fidx is None:
                    raise Exception(
                        'Registered port for {} not detected!'.format(name))
                kwargs = {"port_id": fidx, "num_out_q": conf_workers}
                self.init_fastpath(**kwargs)

            # Initialize kernel slowpath port and RX/TX modules
            try:
                peer = peer_by_interface(name)
                vdev = "net_af_packet{},iface={}".format(idx, peer)
                slow = PMDPort(name="{}Slow".format(name), vdev=vdev)
                spi = __bess_module__("{}SlowPI".format(name), 'PortInc', port=slow.name)
                spo = __bess_module__("{}SlowPO".format(name), 'PortOut', port=slow.name)
                qspo = __bess_module__("{}QSlowPO".format(name), 'Queue')

                # host_ip_filter: tcpdump -i foo 'dst host 198.19.0.1 or 198.18.0.1' -d
                # Should always be set to lowest priority
                HostGate = MAX_GATES - 1
                ips = ips_by_interface(name)
                host_ip_filter = {"priority": -HostGate, "filter": "dst host "
                                + " or ".join(str(x) for x in ips), "gate": HostGate}

                self.bpf.add(filters=[host_ip_filter])

                # Direct control traffic from DPDK to kernel
                link_modules("{}FastBPF".format(name), "{}QSlowPO".format(name), HostGate)
                link_modules("{}QSlowPO".format(name), "{}SlowPO".format(name))

                # Direct control traffic from kernel to DPDK
                link_modules("{}SlowPI".format(name), "{}FastPO".format(name))

                tc = 'slow{}'.format(wid)
                try:
                    bess.add_tc(tc, policy='round_robin', wid=wid)
                except Exception as e:
                    if e.errmsg == "Name '{}' already exists".format(tc):
                        pass
                    else:
                        raise e
                # Limit scheduling slow path RX/TX to 1000 times/second each
                for mod in spi, qspo:
                    bess.add_tc(mod.name,
                            parent=tc,
                            policy='rate_limit',
                            resource='count',
                            limit={'count': 1000})
                    mod.attach_task(mod.name)
            except Exception as e:
                print('Mirror veth interface: {} misconfigured: {}'.format(name, e))
        else:
            raise Exception('Invalid mode selected.')
