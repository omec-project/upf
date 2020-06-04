# vim: syntax=py
# -*- mode: python -*-
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

from conf.parser import *
import conf
import inspect
import sys
import scapy.all as scapy


def setup_globals():
    caller_frame = inspect.stack()[1][0]
    caller_globals = caller_frame.f_globals
    globals().update(caller_globals)


# ====================================================
#       Sim Create Packets
# ====================================================
# Craft a packet with the specified IP addresses
def gen_inet_packet(proto, src_ip, dst_ip):
    eth = scapy.Ether(src='a0:b0:c0:d0:e0:f0', dst='a0:b0:c0:d0:e0:f1')
    ip = scapy.IP(src=src_ip, dst=dst_ip)
    udp = proto(sport=10001, dport=10002)
    payload = 'helloworld'
    pkt = eth/ip/udp/payload
    return bytes(pkt)

def gen_ue_packet(proto, src_ip, dst_ip):
    eth = scapy.Ether(src='a0:b0:c0:d0:e0:f1', dst='a0:b0:c0:d0:e0:f0')
    ip = scapy.IP(src=src_ip, dst=dst_ip)
    udp = proto(sport=2152, dport=2152)
    payload = 'helloworld'
    pkt = eth/ip/udp/payload
    return bytes(pkt)

inet_packets = [gen_inet_packet(scapy.UDP, '172.16.100.1', '16.0.0.1'),
               gen_inet_packet(scapy.UDP, '172.12.55.99', '16.0.0.1')]
ue_packets = [gen_ue_packet(scapy.UDP, '11.1.1.128', '172.1.1.1'),
              gen_ue_packet(scapy.UDP, '11.1.1.129', '182.0.0.2')]


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
    def __init__(self, name, ext_addrs):
        self.name = name
        self.wid = None
        self.fpi = None
        self.fpo = None
        self.bpf = None
        self.rtr = None
        self.rewrite = None
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
        self.fpi = PortInc(name="{}FastPI".format(name), port=fast.name)
        self.fpo = PortOut(name="{}FastPO".format(name), port=fast.name)

        # Initialize BPF to classify incoming traffic to go to kernel and/or pipeline
        self.bpf = BPF(name="{}FastBPF".format(name))
        self.bpf.clear()

        # Initialize route module
        self.rtr = IPLookup(name="{}Routes".format(name))

        # Default route goes to Sink
        self.rtr.add(prefix='0.0.0.0', prefix_len=0, gate=MAX_GATES-1)
        s = Sink(name="{}bad_route".format(name))
        self.rtr.connect(next_mod=s, ogate=MAX_GATES-1)

        # Attach fastpath to worker's root TC
        self.fpi.attach_task(wid=self.wid)

    def init_port(self, idx, conf_mode, conf_workers):
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

        elif conf_mode == 'sim':
            self.fpi = Source(name="{}_source".format(name))
            self.fpo = Sink(name="{}_out".format(name))
            self.bpf = BPF(name="{}FastBPF".format(name))
            self.bpf.clear()

            # Attach fastpath to worker's root TC
            self.fpi.attach_task(wid=self.wid)

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

    def setup_port(self, conf_frag_mtu, conf_measure, conf_mode):
        out = self.fpo
        # enable frag module (if enabled) to control port MTU size
        if conf_frag_mtu is not None:
            frag = IPFrag(name="{}IP4Frag".format(self.name), mtu=conf_frag_mtu)
            frag.connect(next_mod=out, ogate=1)
            frag.connect(next_mod=Sink())
            out = frag

        # create rewrite module if conf_mode == 'sim'
        if conf_mode == 'sim':
            self.rewrite = Rewrite(name="{}_rewrite".format(self.name), templates=inet_packets)
            #self.fpi.connect(next_mod=self.rewrite)

        # enable telemetrics (if enabled) (how many bytes seen in and out of port)
        if conf_measure:
            t = Timestamp(name="{}_timestamp".format(self.name), attr_name="{}timestamp".format(self.name))
            if conf_mode == 'sim':
                self.rewrite.connect(next_mod=t)
            else:
                self.fpi.connect(next_mod=t)
            t.connect(next_mod=self.bpf)
            m = Measure(name="{}_measure".format(self.name), attr_name="{}timestamp".format(self.name))
            m.connect(next_mod=out)
            out = m
        else:
            if conf_mode == 'sim':
                self.rewrite.connect(next_mod=self.bpf)
            else:
                self.fpi.connect(next_mod=self.bpf)

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

        # Direct fast path traffic to Merge module
        merge = Merge(name="{}Merge".format(self.name))

        # Attach Merge module to the 'outlist' of modules
        merge.connect(out)

        if conf_mode == 'sim':
            self.rtr = merge
