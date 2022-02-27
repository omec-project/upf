# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

import time
from ipaddress import IPv4Address
from pprint import pprint

from pkt_utils import GTPU_PORT
from trex_test import TrexTest
from grpc_test import *

from trex_stl_lib.api import (
    STLVM,
    STLPktBuilder,
    STLStream,
    STLTXCont,
)
import ptf.testutils as testutils

UPF_DEST_MAC = "0c:c4:7a:19:6d:ca"

# Port setup
TREX_SENDER_PORT = 0
TREX_RECEIVER_PORT = 1
BESS_SENDER_PORT = 2
BESS_RECEIVER_PORT = 3

# Test specs
DURATION = 10
RATE = 100_000  # 100 Kpps
UE_COUNT = 10_000 # 10k UEs
PKT_SIZE = 64

class PerFlowQosMetricsTest(TrexTest, GrpcTest):
    """
    Generates 1 Mpps downlink traffic for 10k dest UE IP addresses. Uses
    BESS-UPF QoS metrics to verify baseline packet loss, latency, and jitter
    results.
    """
    @autocleanup
    def runTest(self):
        n3TEID = 0

        startIP = IPv4Address('16.0.0.1')
        endIP = startIP + UE_COUNT - 1

        accessIP = IPv4Address('10.128.13.29')
        enbIP = IPv4Address('10.27.19.99') # arbitrary ip for non-existent eNodeB for gtpu encap

        # program UPF for downlink traffic by installing PDRs and FARs
        print("Installing PDRs and FARs...")
        for i in range(UE_COUNT):
            # install N6 DL PDR to match UE dst IP
            pdrDown = self.createPDR(
                srcIface = CORE,
                dstIP = int(startIP + i),
                srcIfaceMask = 0xFF,
                dstIPMask = 0xFFFFFFFF,
                precedence = 255,
                fseID = n3TEID + i + 1, # start from 1
                ctrID = 0,
                farID = i,
                qerIDList = [N6, 1],
                needDecap = 0,
            )
            self.addPDR(pdrDown)

            # install N6 DL FAR for encap
            farDown = self.createFAR(
                farID = i,
                fseID = n3TEID + i + 1, # start from 1
                applyAction = ACTION_FORWARD,
                dstIntf = DST_ACCESS,
                tunnelType = 0x1,
                tunnelIP4Src = int(accessIP),
                tunnelIP4Dst = int(enbIP), # only one eNB to send to downlink
                tunnelTEID = 0,
                tunnelPort = GTPU_PORT,
            )
            self.addFAR(farDown)

            # install N6 DL/UL application QER
            qer = self.createQER(
                gate = GATE_UNMETER,
                qerID = N6,
                fseID = n3TEID + i + 1, # start from 1
                qfi = 9,
                ulGbr = 0,
                ulMbr = 0,
                dlGbr = 0,
                dlMbr = 0,
                burstDurationMs = 10,
            )
            self.addApplicationQER(qer)

        # set up trex to send traffic thru UPF
        print("Setting up TRex client...")
        vm = STLVM()
        vm.var(
            name="dst",
            min_value=str(startIP),
            max_value=str(endIP),
            size=4,
            op="random",
        )
        vm.write(fv_name="dst", pkt_offset="IP.dst")
        vm.fix_chksum()

        pkt = testutils.simple_udp_packet(
            pktlen=PKT_SIZE,
            eth_dst=UPF_DEST_MAC,
            with_udp_chksum=False,
        )
        stream = STLStream(
            packet=STLPktBuilder(pkt=pkt, vm=vm),
            mode=STLTXCont(pps=RATE),
        )
        self.trex_client.add_streams(stream, ports=[BESS_SENDER_PORT])

        print("Running traffic...")
        s_time = time.time()
        self.trex_client.start(
            ports=[BESS_SENDER_PORT], mult="1", duration=DURATION
        )

        # FIXME: pull QoS metrics at end instead of while traffic running
        time.sleep(DURATION - 5)
        if self.trex_client.is_traffic_active():
            stats = self.getSessionStats(q=[90, 99, 99.9], quiet=True)

            preQos = stats["preQos"]
            postDlQos = stats["postDlQos"]
            postUlQos = stats["postUlQos"]

        self.trex_client.wait_on_traffic(ports=[BESS_SENDER_PORT])
        print(f"Duration was {time.time() - s_time}")
        trex_stats = self.trex_client.get_stats()

        sent_packets = trex_stats['total']['opackets']
        recv_packets = trex_stats['total']['ipackets']

        # 0% packet loss
        self.assertEqual(
            sent_packets,
            recv_packets,
            f"Didn't receive all packets; sent {sent_packets}, received {recv_packets}",
        ) 

        for fseid in postDlQos:
            lat = fseid['latency']['percentileValuesNs']
            jitter = fseid['jitter']['percentileValuesNs']

            # 99th %ile latency < 100 us
            self.assertLessEqual(
                int(lat[1]) / 1000,
                100,
                f"99th %ile latency was higher than 100 us! Was {int(lat[1]) / 1000} us"
            )

            # 99.9th %ile latency < 200 us
            self.assertLessEqual(
                int(lat[2]) / 1000,
                200,
                f"99.9th %ile latency was higher than 200 us! Was {int(lat[2]) / 1000} us"
            )

            # 99th% jitter < 100 us
            self.assertLessEqual(
                int(jitter[1]) / 1000,
                100,
                f"99th %ile jitter was higher than 100 us! Was {int(jitter[1]) / 1000} us"  
            )

        return
