# Copyright 2021-present Open Networking Foundation
# SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

import time
from ipaddress import IPv4Address
from pprint import pprint

from trex_test import TrexTest
from grpc_test import GrpcTest, autocleanup

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

class DownlinkBaselineTest(TrexTest, GrpcTest):
    """
    Baseline linerate test generating downlink traffic at 1 Mpps with
    10k UEs and asserting expected performance of BESS-UPF
    """

    @autocleanup
    def runTest(self):
        # test specs
        testDuration = 10
        numSessions = 10_000 # 10k UEs
        tunnelGTPUPort = 2152
        transmitRate = 1_000_000  # 1 Mpps

        n3TEID = 0

        startIP = IPv4Address('16.0.0.1')
        endIP = startIP + numSessions - 1

        accessIP = IPv4Address('10.128.13.29')
        enbIP = IPv4Address('10.27.19.99') # arbitrary ip for nonexistent enodeB

        # program UPF for downlink traffic by installing PDRs and FARs
        print("Installing PDRs and FARs...")
        for i in range(numSessions):
            # install N6 DL PDR to match UE dst IP
            pdrDown = self.createPDR(
                srcIface = self.core,
                dstIP = int(startIP + i),
                srcIfaceMask = 0xFF,
                dstIPMask = 0xFFFFFFFF,
                precedence = 255,
                fseID = n3TEID + i + 1, # start from 1
                ctrID = 0,
                farID = i,
                qerIDList = [self.n6, 1],
                needDecap = 0,
            )
            self.addPDR(pdrDown)

            # install N6 DL FAR for encap
            farDown = self.createFAR(
                farID = i,
                fseID = n3TEID + i + 1, # start from 1
                applyAction = self.actionForward,
                dstIntf = self.dstAccess,
                tunnelType = 0x1,
                tunnelIP4Src = int(accessIP),
                tunnelIP4Dst = int(enbIP), # only one eNB to send to downlink
                tunnelTEID = 0,
                tunnelPort = tunnelGTPUPort,
            )
            self.addFAR(farDown)

            # install N6 DL/UL application QER
            qer = self.createQER(
                gate = self.gateUnmeter,
                qerID = self.n6,
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
            pktlen=64,
            eth_dst=UPF_DEST_MAC,
            with_udp_chksum=False,
        )
        stream = STLStream(
            packet=STLPktBuilder(pkt=pkt, vm=vm),
            mode=STLTXCont(pps=transmitRate),
        )
        self.trex_client.add_streams(stream, ports=[BESS_SENDER_PORT])

        print("Running traffic...")
        s_time = time.time()
        self.trex_client.start(
            ports=[BESS_SENDER_PORT], mult="1", duration=testDuration
        )

        # pull BESS stats once for per-flow latency stats
        time.sleep(5)
        if self.trex_client.is_traffic_active():
            stats = self.getSessionStats(quiet=True)

            preQos = stats["preQos"]
            postDlQos = stats["postDlQos"]
            postUlQos = stats["postUlQos"]

        self.trex_client.wait_on_traffic(ports=[BESS_SENDER_PORT])
        print(f"Duration was {time.time() - s_time}")
        trex_stats = self.trex_client.get_stats()
        pprint(trex_stats)

        # Verify
        # - 99.9th %ile latency < 100 us
        # - 99th %ile jitter < 20 us
        # - 0% packet loss
        sent_packets = trex_stats['total']['opackets']
        recv_packets = trex_stats['total']['ipackets']

        self.assertEqual(
            sent_packets,
            recv_packets,
            f"Didn't receive all packets; sent {sent_packets}, received {recv_packets}",
        ) 

        # Assert latency on downlink fseid's
        for fseid in postDlQos:
            lat = fseid['latency']['percentileValuesNs']
            jitter = fseid['jitter']['percentileValuesNs']

            # assert 99.9th% latency < 100 us
            self.assertLessEqual(
                int(lat[2]) / 1000,
                100,
                f"99.9th %ile was not less than 100 us! Was {int(lat[2]) / 1000}"
            )

            # assert 99th% jitter < 20 us
            self.assertLessEqual(
                int(jitter[1]) / 1000,
                20,
                f"99th %ile jitter was not less than 20 us! Was {int(jitter[1]) / 1000}"  
            )

        return

# TODO
class UplinkBaselineTest(TrexTest, GrpcTest):
    """
    Baseline linerate test generating uplink traffic at 1 Mpps with
    10k UEs and asserting expected performance of BESS-UPF
    """

    @autocleanup
    def runTest(self):
        return
