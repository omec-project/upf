#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

from conf.parser import *
import errno
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
    def __init__(self, name, hwcksum, ext_addrs):
        self.name = name
        self.flow_profiles = []
        self.workers = None
        self.num_q = 1
        self.fpi = None
        self.fpo = None
        self.bpf = None
        self.rtr = None
        self.bpfgate = 0
        self.routes_table = None
        self.nat = None
        self.ext_addrs = ext_addrs
        self.mode = None
        self.hwcksum = hwcksum
        self.cndp_jsonc_file = ""
        self.cndp_lport_start_index = 0

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

    def configure_flow_profiles(self, iface):
        if iface == "access":
            self.flow_profiles = [3]
        if iface == "core":
            self.flow_profiles = [6, 9]

    def configure_cndp(self, jsonc_file, iface, num_workers):
        self.cndp_jsonc_file = jsonc_file
        if iface == "access":
            self.cndp_lport_start_index = 0
        elif iface == "core":
            self.cndp_lport_start_index = num_workers
        else:
            raise Exception('Unknown interface {}: '.format(iface))

    def init_datapath(self, cndp=False, **kwargs):
        # Initialize PMDPort and RX/TX modules
        name = self.name
        if not cndp:
            fast = PMDPort(name="{}Fast".format(name), **kwargs)

        self.fpi = Merge(name="{}PortMerge".format(name))
        self.fpo = WorkerSplit(name="{}QSplit".format(name))

        for qid in range(self.num_q):
            qid_val = qid
            if cndp:
                lport_index = self.cndp_lport_start_index + qid
                fast = CndpPort(jsonc_file=self.cndp_jsonc_file, lport_index=lport_index)
                qid_val = 0

            fpi = QueueInc(name="{}Q{}FastPI".format(name, qid), port=fast.name, qid=qid_val)
            fpi.connect(next_mod=self.fpi)
            # Attach datapath to worker's root TC
            fpi.attach_task(wid=qid)

            fpo = QueueOut(name="{}Q{}FastPO".format(name, qid), port=fast.name, qid=qid_val)
            self.fpo.connect(next_mod=fpo, ogate=qid)


        # Initialize BPF to classify incoming traffic to go to kernel and/or pipeline
        self.bpf = BPF(name="{}FastBPF".format(name))
        self.bpf.clear()

        # Initialize route module
        self.rtr = IPLookup(name="{}Routes".format(name))

        # Default route goes to Sink
        self.rtr.add(prefix='0.0.0.0', prefix_len=0, gate=MAX_GATES-1)
        s = Sink(name="{}bad_route".format(name))
        self.rtr.connect(next_mod=s, ogate=MAX_GATES-1)

    def init_port(self, idx, conf_mode):

        name = self.name
        num_q = len(self.workers)
        self.num_q = num_q
        print('Setting up port {} on worker ids {}'.format(name, self.workers))

        # Detect the mode of this interface - DPDK/AF_XDP/AF_PACKET
        if conf_mode is None:
            conf_mode = self.detect_mode()

        if conf_mode not in ['af_xdp', 'linux', 'dpdk', 'af_packet', 'sim', 'cndp']:
            raise Exception('Invalid mode: {} selected.'.format(conf_mode))

        if conf_mode in ['af_xdp', 'linux']:
            try:
                # Initialize kernel datapath.
                # AF_XDP requires that num_rx_qs == num_tx_qs
                kwargs = {"vdev" : "net_af_xdp{},iface={},start_queue=0,queue_count={}"
                          .format(idx, name, num_q), "num_out_q": num_q, "num_inc_q": num_q}
                self.init_datapath(**kwargs)
            except:
                if conf_mode == 'linux':
                    print('Failed to create AF_XDP socket for {}. Retrying with AF_PACKET socket...'.format(name))
                    conf_mode = 'af_packet'
                else:
                    print('Failed to create AF_XDP socket for {}. Exiting...'.format(name))
                    sys.exit()

        if conf_mode == 'cndp':
            try:
                # Initialize kernel fastpath.
                self.init_datapath(cndp=True)
            except:
                print('Failed to create CNDP/AF_XDP socket for {}. Exiting...'.format(name))
                sys.exit()

        if conf_mode == 'af_packet':
            try:
                # Initialize kernel datapath
                kwargs = {"vdev" : "net_af_packet{},iface={},qpairs={}"
                          .format(idx, name, num_q), "num_out_q": num_q, "num_inc_q": num_q}
                self.init_datapath(**kwargs)
            except:
                print('Failed to create AF_PACKET socket for {}. Exiting...'.format(name))
                sys.exit()

        if conf_mode == 'sim':
            self.fpi = Source(name="{}_source".format(name))
            self.fpo = Sink(name="{}_out".format(name))
            self.bpf = BPF(name="{}FastBPF".format(name))
            self.bpf.clear()

            # Attach datapath to worker's root TC
            self.fpi.attach_task(wid=0)

        if conf_mode == 'dpdk':
            kwargs = None
            pci = alias_by_interface(name)
            if pci is not None:
                kwargs = {"pci": pci, "num_out_q": num_q, "num_inc_q": num_q, "hwcksum": self.hwcksum, "flow_profiles": self.flow_profiles}
                try:
                    self.init_datapath(**kwargs)
                except:
                    kwargs = None
                    print('Unable to initialize {} datapath using alias {},\
                        falling back to scan'.format(name, pci))
            if kwargs is None:
                # Fallback to scanning ports
                # if port list is empty, scan for dpdk_ports first
                if not dpdk_ports and scan_dpdk_ports() == False:
                    print('Registered dpdk ports do not exist.')
                    sys.exit()
                # Initialize DPDK datapath
                fidx = dpdk_ports.get(mac_by_interface(name))
                if fidx is None:
                    raise Exception(
                        'Registered port for {} not detected!'.format(name))
                kwargs = {"port_id": fidx, "num_out_q": num_q, "num_inc_q": num_q, "hwcksum": self.hwcksum, "flow_profiles": self.flow_profiles}
                self.init_datapath(**kwargs)

            # Initialize kernel slowpath port and RX/TX modules
            try:
                peer = peer_by_interface(name)
                vdev = "net_af_packet{},iface={}".format(idx, peer)
                slow = PMDPort(name="{}Slow".format(name), vdev=vdev)
                spi = PortInc(name="{}SlowPI".format(name), port=slow.name)
                spo = PortOut(name="{}SlowPO".format(name), port=slow.name)
                qspo = Queue(name="{}QSlowPO".format(name))

                # host_ip_filter: tcpdump -i foo 'dst host 198.19.0.1 or 198.18.0.1' -d
                # Should always be set to lowest priority
                HostGate = MAX_GATES - 1
                ips = ips_by_interface(name)
                host_ip_filter = {"priority": -HostGate, "filter": "dst host "
                                + " or ".join(str(x) for x in ips), "gate": HostGate}

                self.bpf.add(filters=[host_ip_filter])

                # Direct control traffic from DPDK to kernel
                self.bpf.connect(next_mod=qspo, ogate=HostGate)
                qspo.connect(next_mod=spo)

                # Direct control traffic from kernel to DPDK
                spi.connect(next_mod=self.fpo)

                tc = 'slow{}'.format(0)
                try:
                    bess.add_tc(tc, policy='round_robin', wid=0)
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

        # Finall set conf mode
        self.mode = conf_mode

    def setup_port(self, conf_frag_mtu, conf_defrag_flows, conf_measure, type_of_packets = "", **seq_kwargs):
        out = self.fpo
        inc = self.fpi
        gate = 0

        # enable frag module (if enabled) to control port MTU size
        if conf_frag_mtu is not None:
            frag = IPFrag(name="{}IP4Frag".format(self.name), mtu=conf_frag_mtu)
            s = Sink(name="{}IP4FragFail".format(self.name))
            frag.connect(next_mod=s)
            frag.connect(next_mod=out, ogate=1)
            out = frag

        # create rewrite module if mode == 'sim'
        if self.mode == 'sim':
            rewrite = Rewrite(name="{}_rewrite".format(self.name), templates=type_of_packets)
            update = SequentialUpdate(name="{}_update".format(self.name), **seq_kwargs)
            udpcsum = L4Checksum()
            ipcsum = IPChecksum()

            self.fpi.connect(next_mod=rewrite)
            rewrite.connect(next_mod=update)
            update.connect(next_mod=udpcsum)
            udpcsum.connect(next_mod=ipcsum)

            inc = ipcsum

        # enable telemetrics (if enabled) (how many bytes seen in and out of port)
        if conf_measure:
            t = Timestamp(name="{}_timestamp".format(self.name))
            inc.connect(next_mod=t)

            m = Measure(name="{}_measure".format(self.name))
            m.connect(next_mod=out)

            out = m
            inc = t

        if conf_defrag_flows is not None:
            defrag = IPDefrag(name="{}IP4Defrag".format(self.name), num_flows=conf_defrag_flows, numa=-1)
            s = Sink(name="{}DefragFail".format(self.name))
            defrag.connect(next_mod=s)
            inc.connect(next_mod=defrag)
            inc = defrag
            gate = 1

        # Connect inc to bpf
        inc.connect(next_mod=self.bpf, ogate=gate)

        # Attach nat module (if enabled)
        if self.ext_addrs is not None:
            # Tokenize the string
            addrs = self.ext_addrs.split(' or ')
            # Make a list of ext_addr
            nat_list = list()
            for addr in addrs:
                nat_dict = dict()
                nat_dict['ext_addr'] = addr
                nat_list.append(nat_dict)

            # Create the NAT module
            self.nat = NAT(name="{}NAT".format(self.name), ext_addrs=nat_list)
            self.nat.connect(next_mod=out, ogate=1)
            out = self.nat

        # Set src mac address on Ethernet header for egress pkts
        update = Update(name="{}SrcEther".format(self.name), fields=[
            {'offset': 6, 'size': 6, 'value': mac2hex(mac_by_interface(self.name))}
            ])

        # Attach Update module to the 'outlist' of modules
        update.connect(out)

        # Direct fast path traffic to Merge module
        merge = Merge(name="{}Merge".format(self.name))

        # Attach it to merge
        merge.connect(update)

        if self.mode == 'sim':
            self.rtr = merge
