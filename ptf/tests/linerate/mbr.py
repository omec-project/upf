# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

from ipaddress import IPv4Address

from trex.stl.trex_stl_streams import STLFlowLatencyStats
from trex_stl_lib.api import (
    STLPktBuilder,
    STLStream,
    STLTXCont,
)

from grpc_test import *
from trex_test import TrexTest
from trex_utils import *

# TODO: move to global constant file (or env)
#  since it depends on server where BESS is running
UPF_DEST_MAC = "0c:c4:7a:19:6d:ca"

# Port setup
TREX_SENDER_PORT = 0
TREX_RECEIVER_PORT = 1
BESS_SENDER_PORT = 2
BESS_RECEIVER_PORT = 3

# test specs
DURATION = 3
GTPU_PORT = 2152
PKT_SIZE = 1400


class DlAppMbrTest(TrexTest, GrpcTest):
    """Base class for dowlink MBR testing"""

    @autocleanup
    def run_dl_traffic(self, mbr_bps, stream_bps, duration) -> FlowStats:
        mbr_kbps = mbr_bps / K
        burst_ms = 10
        teid = 1
        ue_ip = IPv4Address('16.0.0.1')
        # TODO: move to global constant file (or env)
        access_ip = IPv4Address('10.128.13.29')
        enb_ip = IPv4Address('10.27.19.99')

        pdrDown = self.createPDR(
            srcIface=CORE,
            dstIP=int(ue_ip),
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
            tunnelIP4Src=int(access_ip),
            tunnelIP4Dst=int(enb_ip),
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
            eth_dst=UPF_DEST_MAC,
            ip_dst=str(ue_ip),
            with_udp_chksum=False
        )
        stream = STLStream(
            packet=STLPktBuilder(pkt=pkt),
            mode=STLTXCont(bps_L2=stream_bps),
            flow_stats=STLFlowLatencyStats(pg_id=0),
        )
        self.trex_client.add_streams(stream, ports=[BESS_SENDER_PORT])

        start_and_monitor_port_stats(
            client=self.trex_client,
            duration=duration,
            tx_port=BESS_SENDER_PORT,
            rx_port=BESS_RECEIVER_PORT,
            min_tx_bps=stream_bps * 0.95)

        trex_stats = self.trex_client.get_stats()
        return get_flow_stats(0, trex_stats)


class DlAppMbrConformingTest(DlAppMbrTest):
    """
    Verifies that traffic conforming to the app MBR is not dropped.
    """

    def runTest(self):
        rates = [10 * M, 50 * M, 100 * M, 200 * M]

        print()
        for mbr_bps in rates:
            print(f"Testing app MBR {to_readable(mbr_bps)}...")
            flow_stats = self.run_dl_traffic(
                mbr_bps=mbr_bps, stream_bps=mbr_bps*0.98, duration=2)

            # 0% packet loss
            self.assertEqual(
                flow_stats.tx_packets,
                flow_stats.rx_packets,
                f"Conforming app streams should not experience drops "
                f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
            )
