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
DURATION = 5
GTPU_PORT = 2152
PKT_SIZE = 1400


class DlAppMbrConformingTest(TrexTest, GrpcTest):
    """
    Verifies that traffic conforming to the app MBR is not dropped.
    """

    @autocleanup
    def runTest(self):

        mbr_bps = 100 * M
        mbr_kbps = mbr_bps / K
        burst_ms = 10
        stream_bps = mbr_bps * 0.99

        teid = 1

        ueIP = IPv4Address('16.0.0.1')
        # TODO: move to global constant file (or env)
        accessIP = IPv4Address('10.128.13.29')
        enbIP = IPv4Address('10.27.19.99')

        pdrDown = self.createPDR(
            srcIface=CORE,
            dstIP=int(ueIP),
            srcIfaceMask=0xFF,
            dstIPMask=0xFFFFFFFF,
            precedence=255,
            fseID=1,
            ctrID=0,
            farID=1,
            qerIDList=[1],
            needDecap=0,
        )
        self.addPDR(pdrDown, True)

        farDown = self.createFAR(
            farID=1,
            fseID=1,
            applyAction=ACTION_FORWARD,
            dstIntf=DST_ACCESS,
            tunnelType=0x1, # Replace with constant
            tunnelIP4Src=int(accessIP),
            tunnelIP4Dst=int(enbIP),
            tunnelTEID=teid,
            tunnelPort=GTPU_PORT,
        )
        self.addFAR(farDown, True)

        qer = self.createQER(
            gate=GATE_METER,
            qerID=1,
            fseID=1,
            ulMbr=mbr_kbps,
            dlMbr=mbr_kbps,
            burstDurationMs=burst_ms,
        )
        self.addApplicationQER(qer, True)

        print("Setting up TRex client...")
        pkt = testutils.simple_udp_packet(
            pktlen=PKT_SIZE,
            eth_dst=UPF_DEST_MAC,
            ip_dst=str(ueIP),
            with_udp_chksum=False
        )
        stream = STLStream(
            packet=STLPktBuilder(pkt=pkt),
            mode=STLTXCont(bps_L2=stream_bps),
            flow_stats=STLFlowLatencyStats(pg_id=0),
        )
        self.trex_client.add_streams(stream, ports=[BESS_SENDER_PORT])

        print("Running traffic...")
        s_time = time.time()
        self.trex_client.start(
            ports=[BESS_SENDER_PORT],
            mult="1",
            duration=DURATION,
        )

        monitor_port_stats(self.trex_client)

        self.trex_client.wait_on_traffic(ports=[BESS_SENDER_PORT])
        print(f"Duration was {time.time() - s_time}")

        trex_stats = self.trex_client.get_stats()
        flow_stats = get_flow_stats(0, trex_stats)

        # TODO: sanity check Trex TX rate
        #  Did we generate enough traffic? should be close to stream_bps

        # 0% packet loss
        self.assertEqual(
            flow_stats.tx_packets,
            flow_stats.rx_packets,
            f"Conforming app streams should not experience drops "
            f"(sent {flow_stats.tx_packets} pkts, received {flow_stats.rx_packets})",
        )
