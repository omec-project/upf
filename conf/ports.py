#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

import errno
import inspect
import sys

from conf.parser import *


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
            intf = PMDPort(
                name=f"Port {idx}",
                port_id=idx,
                num_inc_q=1,
                num_out_q=1,
            )
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
    return bool(dpdk_ports)


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

    def bpf_gate(self):
        if self.bpfgate < MAX_GATES - 2:
            self.bpfgate += 1
            return self.bpfgate
        else:
            raise RuntimeError(f"Port {self.name}: Out of BPF gates to allocate")

    def detect_mode(self):
        mode = None
        try:
            peer_by_interface(self.name)
            mode = "dpdk"
        except LookupError:
            mode = "linux"
        return mode

    def configure_flow_profiles(self, iface):
        if iface == "access":
            self.flow_profiles = [3]
        if iface == "core":
            self.flow_profiles = [6, 9]

    def init_datapath(self, **kwargs):
        # Initialize PMDPort and RX/TX modules
        name = self.name
        fast = PMDPort(name=f"{name}Fast", **kwargs)

        self.fpi = Merge(name=f"{name}PortMerge")
        self.fpo = WorkerSplit(name=f"{name}QSplit")

        for qid in range(self.num_q):
            fpi = QueueInc(name=f"{name}Q{qid}FastPI", port=fast.name, qid=qid)
            fpi.connect(next_mod=self.fpi)
            # Attach datapath to worker's root TC
            fpi.attach_task(wid=qid)

            fpo = QueueOut(name=f"{name}Q{qid}FastPO", port=fast.name, qid=qid)
            self.fpo.connect(next_mod=fpo, ogate=qid)

        # Initialize BPF to classify incoming traffic to go to kernel and/or pipeline
        self.bpf = BPF(name=f"{name}FastBPF")
        self.bpf.clear()

        # Initialize route module
        self.rtr = IPLookup(name=f"{name}Routes")

        # Default route goes to Sink
        self.rtr.add(prefix="0.0.0.0", prefix_len=0, gate=MAX_GATES - 1)
        s = Sink(name=f"{name}bad_route")
        self.rtr.connect(next_mod=s, ogate=MAX_GATES - 1)

    def init_port(self, idx, conf_mode):
        name = self.name
        num_q = len(self.workers)
        self.num_q = num_q
        print(f"Setting up port {name} on worker ids {self.workers}")

        # Detect the mode of this interface - DPDK/AF_XDP/AF_PACKET
        if conf_mode is None:
            conf_mode = self.detect_mode()

        if conf_mode not in ["af_xdp", "linux", "dpdk", "af_packet", "sim"]:
            raise ValueError(f"Invalid mode: {conf_mode} selected.")

        if conf_mode in ["af_xdp", "linux"]:
            try:
                # Initialize kernel datapath.
                # AF_XDP requires that num_rx_qs == num_tx_qs
                kwargs = {
                    "vdev": f"net_af_xdp{idx},iface={name},start_queue=0,queue_count={num_q}",
                    "num_out_q": num_q,
                    "num_inc_q": num_q,
                }
                self.init_datapath(**kwargs)
            except bess.Error:
                if conf_mode == "linux":
                    print(
                        f"Failed to create AF_XDP socket for {name}. Retrying with AF_PACKET socket..."
                    )
                    conf_mode = "af_packet"
                else:
                    print(f"Failed to create AF_XDP socket for {name}. Exiting...")
                    sys.exit()

        if conf_mode == "af_packet":
            try:
                # Initialize kernel datapath
                kwargs = {
                    "vdev": f"net_af_packet{idx},iface={name},qpairs={num_q}",
                    "num_out_q": num_q,
                    "num_inc_q": num_q,
                }
                self.init_datapath(**kwargs)
            except bess.Error:
                print(f"Failed to create AF_PACKET socket for {name}. Exiting...")
                sys.exit()

        if conf_mode == "sim":
            self.fpi = Source(name=f"{name}_source")
            self.fpo = Sink(name=f"{name}_out")
            self.bpf = BPF(name=f"{name}FastBPF")
            self.bpf.clear()

            # Attach datapath to worker's root TC
            self.fpi.attach_task(wid=0)

        if conf_mode == "dpdk":
            kwargs = None
            pci = alias_by_interface(name)
            if pci is not None:
                kwargs = {
                    "pci": pci,
                    "num_out_q": num_q,
                    "num_inc_q": num_q,
                    "hwcksum": self.hwcksum,
                    "flow_profiles": self.flow_profiles,
                }
                try:
                    self.init_datapath(**kwargs)
                except bess.Error as err:
                    kwargs = None
                    print(
                        f"Unable to initialize {name} datapath using alias {pci}: {err}. \
                        Falling back to port_id/scan"
                    )
            if kwargs is None:
                try:
                    kwargs = {
                        "port_id": idx,
                        "num_out_q": num_q,
                        "num_inc_q": num_q,
                        "hwcksum": self.hwcksum,
                        "flow_profiles": self.flow_profiles,
                    }
                    self.init_datapath(**kwargs)
                except bess.Error as err:
                    kwargs = None
                    print(
                        f"Unable to initialize {name} datapath using port_id {idx}: {err}. \
                        Falling back to scan"
                    )

            if kwargs is None:
                # Fallback to scanning ports
                # if port list is empty, scan for dpdk_ports first
                if not dpdk_ports and scan_dpdk_ports() == False:
                    print("Registered dpdk ports do not exist.")
                    sys.exit()
                # Initialize DPDK datapath
                fidx = dpdk_ports.get(mac_by_interface(name))
                if fidx is None:
                    raise LookupError(f"Registered port for {name} not detected!")
                kwargs = {
                    "port_id": fidx,
                    "num_out_q": num_q,
                    "num_inc_q": num_q,
                    "hwcksum": self.hwcksum,
                    "flow_profiles": self.flow_profiles,
                }
                self.init_datapath(**kwargs)

            # Initialize kernel slowpath port and RX/TX modules
            try:
                peer = peer_by_interface(name)
                vdev = f"net_af_packet{idx},iface={peer}"
                slow = PMDPort(name=f"{name}Slow", vdev=vdev)
                spi = PortInc(name=f"{name}SlowPI", port=slow.name)
                spo = PortOut(name=f"{name}SlowPO", port=slow.name)
                qspo = Queue(name=f"{name}QSlowPO")

                # host_ip_filter: tcpdump -i foo 'dst host 198.19.0.1 or 198.18.0.1' -d
                # Should always be set to lowest priority
                HostGate = MAX_GATES - 1
                ips = ips_by_interface(name)
                host_ip_filter = {
                    "priority": -HostGate,
                    "filter": "dst host " + " or ".join(str(x) for x in ips),
                    "gate": HostGate,
                }

                self.bpf.add(filters=[host_ip_filter])

                # Direct control traffic from DPDK to kernel
                self.bpf.connect(next_mod=qspo, ogate=HostGate)
                qspo.connect(next_mod=spo)

                # Direct control traffic from kernel to DPDK
                spi.connect(next_mod=self.fpo)

                tc = f"slow{0}"
                try:
                    bess.add_tc(tc, policy="round_robin", wid=0)
                except bess.Error as e:
                    if e.errmsg == f"Name '{tc}' already exists":
                        pass
                    else:
                        raise
                # Limit scheduling slow path RX/TX to 1000 times/second each
                for mod in spi, qspo:
                    bess.add_tc(
                        mod.name,
                        parent=tc,
                        policy="rate_limit",
                        resource="count",
                        limit={"count": 1000},
                    )
                    mod.attach_task(mod.name)
            except bess.Error as e:
                print(f"Mirror veth interface: {name} misconfigured: {e}")

        # Finall set conf mode
        self.mode = conf_mode

    def setup_port(
        self,
        conf_frag_mtu,
        conf_defrag_flows,
        conf_measure,
        type_of_packets="",
        **seq_kwargs,
    ):
        out = self.fpo
        inc = self.fpi
        gate = 0

        # enable frag module (if enabled) to control port MTU size
        if conf_frag_mtu is not None:
            frag = IPFrag(name=f"{self.name}IP4Frag", mtu=conf_frag_mtu)
            s = Sink(name=f"{self.name}IP4FragFail")
            frag.connect(next_mod=s)
            frag.connect(next_mod=out, ogate=1)
            out = frag

        # create rewrite module if mode == 'sim'
        if self.mode == "sim":
            rewrite = Rewrite(name=f"{self.name}_rewrite", templates=type_of_packets)
            update = SequentialUpdate(name=f"{self.name}_update", **seq_kwargs)
            udpcsum = L4Checksum()
            ipcsum = IPChecksum()

            self.fpi.connect(next_mod=rewrite)
            rewrite.connect(next_mod=update)
            update.connect(next_mod=udpcsum)
            udpcsum.connect(next_mod=ipcsum)

            inc = ipcsum

        # enable telemetrics (if enabled) (how many bytes seen in and out of port)
        if conf_measure:
            t = Timestamp(name=f"{self.name}_timestamp")
            inc.connect(next_mod=t)

            m = Measure(name=f"{self.name}_measure")
            m.connect(next_mod=out)

            out = m
            inc = t

        if conf_defrag_flows is not None:
            defrag = IPDefrag(
                name=f"{self.name}IP4Defrag",
                num_flows=conf_defrag_flows,
                numa=-1,
            )
            s = Sink(name=f"{self.name}DefragFail")
            defrag.connect(next_mod=s)
            inc.connect(next_mod=defrag)
            inc = defrag
            gate = 1

        # Connect inc to bpf
        inc.connect(next_mod=self.bpf, ogate=gate)

        # Attach nat module (if enabled)
        if self.ext_addrs is not None:
            # Tokenize the string
            addrs = self.ext_addrs.split(" or ")
            # Make a list of ext_addr
            nat_list = []
            for addr in addrs:
                nat_dict = {"ext_addr": addr}
                nat_list.append(nat_dict)

            # Create the NAT module
            self.nat = NAT(name=f"{self.name}NAT", ext_addrs=nat_list)
            self.nat.connect(next_mod=out, ogate=1)
            out = self.nat

        # Set src mac address on Ethernet header for egress pkts
        update = Update(
            name=f"{self.name}SrcEther",
            fields=[
                {"offset": 6, "size": 6, "value": mac2hex(mac_by_interface(self.name))}
            ],
        )

        # Attach Update module to the 'outlist' of modules
        update.connect(out)

        # Direct fast path traffic to Merge module
        merge = Merge(name=f"{self.name}Merge")

        # Attach it to merge
        merge.connect(update)

        if self.mode == "sim":
            self.rtr = merge
