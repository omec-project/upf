# SPDX-License-Identifier: Apache-2.0
# Copyright 2022-present Open Networking Foundation

from ipaddress import IPv4Address

from scapy.layers.l2 import Ether
from trex.stl.trex_stl_streams import STLFlowLatencyStats
from trex_stl_lib.api import (
    STLPktBuilder,
    STLStream,
    STLTXCont,
)

from grpc_test import *
from pkt_utils import GTPU_PORT, pkt_add_gtpu
from trex_test import TrexTest
from trex_utils import *

# TODO: move to global constant file (or env)
#  since it depends on Trex config and server where BESS is running
# Port setup
BESS_CORE_MAC = "0c:c4:7a:19:6d:ca"
BESS_ACCESS_MAC = "0c:c4:7a:19:6d:cb"
BESS_CORE_PORT = 2
BESS_ACCESS_PORT = 3

# test specs
DURATION = 3
PKT_SIZE = 1400

# TODO: move to global constant file (or env)
UE_IP = IPv4Address('16.0.0.1')
ENB_IP = IPv4Address('10.27.19.99')
N3_IP = IPv4Address('10.128.13.29')
# Must be routable by route_control
PDN_IP = IPv4Address("11.1.1.129")


class AppMbrTest(TrexTest, GrpcTest):
    """Base class for app MBR testing"""

    @autocleanup
    def run_dl_traffic(self, mbr_bps, stream_bps, num_samples) -> FlowStats:
        mbr_kbps = mbr_bps / K
        burst_ms = 10
        teid = 1

        pdrDown = self.createPDR(
            srcIface=CORE,
            dstIP=int(UE_IP),
            srcIfaceMask=0xFF,
            dstIPMask=0xFFFFFFFF,
            precedence=255,
            fseID=1,
            ctrID=0,
            farID=1,
            qerIDList=[1],
            needDecap=0,
        )
        self.addPDR(pdrDown)

        farDown = self.createFAR(
            farID=1,
            fseID=1,
            applyAction=ACTION_FORWARD,
            dstIntf=DST_ACCESS,
            tunnelType=0x1,  # Replace with constant
            tunnelIP4Src=int(N3_IP),
            tunnelIP4Dst=int(ENB_IP),
            tunnelTEID=teid,
            tunnelPort=GTPU_PORT,
        )
        self.addFAR(farDown)

        qer = self.createQER(
            gate=GATE_METER,
            qerID=1,
            fseID=1,
            ulMbr=mbr_kbps,
            dlMbr=mbr_kbps,
            burstDurationMs=burst_ms,
        )
        self.addApplicationQER(qer)

        pkt = testutils.simple_udp_packet(
            pktlen=PKT_SIZE,
            eth_dst=BESS_CORE_MAC,
            ip_dst=str(UE_IP),
            with_udp_chksum=False
        )
        app_payload_size = len(pkt[Ether].payload)

        overhead = len(pkt) / app_payload_size
        stream_bps = overhead * stream_bps
        print(f" TX rate with Ethernet overhead: {to_readable(stream_bps)} ({overhead:.1%})")
        stream = STLStream(
            packet=STLPktBuilder(pkt=pkt),
            mode=STLTXCont(bps_L2=stream_bps),
            flow_stats=STLFlowLatencyStats(pg_id=0),
        )
        self.trex_client.add_streams(stream, ports=[BESS_CORE_PORT])

        start_and_monitor_port_stats(
            client=self.trex_client,
            num_samples=num_samples,
            tx_port=BESS_CORE_PORT,
            rx_port=BESS_ACCESS_PORT,
            min_tx_bps=stream_bps * 0.95)

        trex_stats = self.trex_client.get_stats()
        return get_flow_stats(0, trex_stats)

    @autocleanup
    def run_ul_traffic(self, mbr_bps, stream_bps, num_samples) -> FlowStats:
        mbr_kbps = mbr_bps / K
        burst_ms = 10
        teid = 1

        pdrUp = self.createPDR(
            srcIface=ACCESS,
            tunnelIP4Dst=int(N3_IP),
            tunnelTEID=teid,
            srcIfaceMask=0xFF,
            tunnelIP4DstMask=0xFFFFFFFF,
            tunnelTEIDMask=0xFFFF,
            precedence=255,
            fseID=1,
            ctrID=0,
            farID=1,
            qerIDList=[1],
            needDecap=1,
        )
        self.addPDR(pdrUp)

        farUp = self.createFAR(
            farID=1,
            fseID=1,
            applyAction=ACTION_FORWARD,
            dstIntf=DST_CORE,
        )
        self.addFAR(farUp)

        qer = self.createQER(
            gate=GATE_METER,
            qerID=1,
            fseID=1,
            ulMbr=mbr_kbps,
            dlMbr=mbr_kbps,
            burstDurationMs=burst_ms,
        )
        self.addApplicationQER(qer)

        pkt = testutils.simple_udp_packet(
            pktlen=PKT_SIZE,
            eth_dst=BESS_ACCESS_MAC,
            ip_src=str(UE_IP),
            ip_dst=str(PDN_IP),
            with_udp_chksum=False
        )
        app_payload_size = len(pkt[Ether].payload)
        gtpu_pkt = pkt_add_gtpu(
            pkt=pkt,
            out_ipv4_src=str(ENB_IP),
            out_ipv4_dst=str(N3_IP),
            teid=teid,
        )

        overhead = len(gtpu_pkt) / app_payload_size
        stream_bps = overhead * stream_bps
        print(f" TX rate with Ethernet+GTPU overhead: {to_readable(stream_bps)} ({overhead:.1%})")
        stream = STLStream(
            packet=STLPktBuilder(pkt=gtpu_pkt),
            mode=STLTXCont(bps_L2=stream_bps),
            flow_stats=STLFlowLatencyStats(pg_id=0),
        )
        self.trex_client.add_streams(stream, ports=[BESS_ACCESS_PORT])

        start_and_monitor_port_stats(
            client=self.trex_client,
            num_samples=num_samples,
            tx_port=BESS_ACCESS_PORT,
            rx_port=BESS_CORE_PORT,
            min_tx_bps=stream_bps * 0.95)

        trex_stats = self.trex_client.get_stats()
        return get_flow_stats(0, trex_stats)


class DlAppMbrConformingTest(AppMbrTest):
    """
    Verifies that downlink traffic conforming to the app MBR is not dropped.
    """

    def runTest(self):
        # Send slightly below the MBR, expect no packet loss.
        mbrs_bps = [10 * M, 50 * M, 100 * M, 200 * M]
        print()
        for mbr_bps in mbrs_bps:
            print(f"Testing app MBR {to_readable(mbr_bps)}...")
            flow_stats = self.run_dl_traffic(
                mbr_bps=mbr_bps, stream_bps=mbr_bps*0.99, num_samples=2)
            self.assertEqual(
                flow_stats.tx_packets,
                flow_stats.rx_packets,
                f"Conforming app streams should not experience drops "
                f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
            )


class DlAppMbrNonConformingTest(AppMbrTest):
    """
    Verifies that downlink traffic non-conforming to the app MBR is policed.
    """

    def runTest(self):
        # Send twice the MBR, expect 50% packet loss.
        mbrs_bps = [10 * M, 50 * M, 100 * M, 200 * M]
        print()
        for mbr_bps in mbrs_bps:
            print(f"Testing app MBR {to_readable(mbr_bps)}...")
            flow_stats = self.run_dl_traffic(
                mbr_bps=mbr_bps, stream_bps=mbr_bps*2, num_samples=2)
            loss = (flow_stats.tx_packets - flow_stats.rx_packets) / flow_stats.tx_packets
            self.assertAlmostEqual(
                loss,
                0.5,
                delta=0.01,
                msg=f"Non-conforming app streams should experience around 50% pkt loss "
                f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
            )


class UlAppMbrConformingTest(AppMbrTest):
    """
    Verifies that uplink traffic conforming to the app MBR is not dropped.
    """

    def runTest(self):
        # Send slightly below the MBR, expect no packet loss.
        mbrs_bps = [10 * M, 50 * M, 100 * M, 200 * M]
        print()
        for mbr_bps in mbrs_bps:
            print(f"Testing app MBR {to_readable(mbr_bps)}...")
            flow_stats = self.run_ul_traffic(
                mbr_bps=mbr_bps, stream_bps=mbr_bps*0.99, num_samples=2)
            self.assertEqual(
                flow_stats.tx_packets,
                flow_stats.rx_packets,
                f"Conforming app streams should not experience drops "
                f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
            )


class UlAppMbrNonConformingTest(AppMbrTest):
    """
    Verifies that uplink traffic non-conforming to the app MBR is policed.
    """

    def runTest(self):
        # Send twice the MBR, expect 50% packet loss.
        mbrs_bps = [10 * M, 50 * M, 100 * M, 200 * M]
        print()
        for mbr_bps in mbrs_bps:
            print(f"Testing app MBR {to_readable(mbr_bps)}...")
            flow_stats = self.run_ul_traffic(
                mbr_bps=mbr_bps, stream_bps=mbr_bps*2, num_samples=2)
            loss = (flow_stats.tx_packets - flow_stats.rx_packets) / flow_stats.tx_packets
            self.assertAlmostEqual(
                loss,
                0.5,
                delta=0.01,
                msg=f"Non-conforming app streams should experience around 50% pkt loss "
                f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
            )
