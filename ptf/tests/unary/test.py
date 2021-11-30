# Copyright 2021-present Open Networking Foundation
# SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

import time
from ipaddress import IPv4Address

from trex_test import TrexTest
from grpc_test import GrpcTest

from trex_stl_lib.api import (
    STLVM,
    STLPktBuilder,
    STLStream,
    STLTXCont,
)
import ptf.testutils as testutils

RATE = 1_000_000_000  # 1 Gbps
UPF_DEST_MAC = "0c:c4:7a:19:6d:ca"

# Port setup
TREX_SENDER_PORT = 0
TREX_RECEIVER_PORT = 1
BESS_SENDER_PORT = 2
BESS_RECEIVER_PORT = 3

class PdrTest(TrexTest, GrpcTest):
    def runTest(self):
        # create basic N6 downlink pdr
        pdr = self.createPDR(
            srcIface = self.core,
            dstIP = int(IPv4Address('16.0.0.1')),
            srcIfaceMask = 0xFF,
            dstIPMask = 0xFFFFFFFF,
            precedence = 255,
            fseID = 0x30000000,
            ctrID = 0,
            farID = self.n3,
            qerIDList = [self.n6, 1],
            needDecap = 0,
        )

        print("add pdr response:")
        print(self.addPDR(pdr))
        print()

        # Testing purposes: verify bess fails to find PDR when modified
        # pdr = pdr._replace(srcIfaceMask=0xAF)
        print("del pdr response:")
        print(self.delPDR(pdr))
        print()


class FarTest(TrexTest, GrpcTest):
    def runTest(self):
        # create basic N6 uplink FAR
        far = self.createFAR(
            farID = self.n6,
            fseID = 0x30000000,
            applyAction = self.actionForward,
            dstIntf = self.dstCore,
        )

        print("add far response:")
        print(self.addFAR(far))
        print()

        # Testing purposes: verify bess fails to find FAR when modified
        # far = far._replace(fseID=0xA0000000)
        print("del far response:")
        print(self.delFAR(far))
        print()
    

class QerTest(TrexTest, GrpcTest):
    def runTest(self):
        qer = self.createQER()

        # configure as basic N6 UL/DL QER
        qer["gate"] = self.gateUnmeter
        qer["qfi"] = 9
        qer["qerID"] = self.n6
        qer["fseID"] = 0x30000000

        qer["ulGbr"] = 50_000
        qer["ulMbr"] = 90_000
        qer["dlGbr"] = 60_000
        qer["dlMbr"] = 80_000
        qer["burstDurationMs"] = 10

        print("add qer response:")
        self.addApplicationQER(qer)
        print()

        print("del qer response:")
        self.delApplicationQER(qer)
        print()


class SimpleTest(TrexTest, GrpcTest):
    def runTest(self):
        # define num UE sessions, start UEIP
        numSessions = 10
        n3TEID = n9TEID = 0
        tunnelGTPUPort = 2152

        startIP = IPv4Address('16.0.0.1')
        endIP = startIP + numSessions - 1

        accessIP = coreIP = IPv4Address('10.128.13.29')
        enbIP = IPv4Address('10.27.19.99') # arbitrary ip for nonexistent enodeB
        # AUPFIP = IPv4Address('27.10.19.99') # ??

        # program UPF for downlink traffic by installing PDRs and FARs
        print("Installing PDRs and FARs...")
        for i in range(numSessions):
            # create N6 UL and DL PDRs for UEIP
            pdrDown = self.createPDR(
                srcIface = self.core,
                dstIP = int(startIP + i),
                srcIfaceMask = 0xFF,
                dstIPMask = 0xFFFFFFFF,
                precedence = 255,
                fseID = n3TEID + i,
                ctrID = 0,
                farID = i,
                qerIDList = [self.n6, 1],
                needDecap = 0,
            )

            # pdrUp = self.createPDR(
            #     srcIface = self.access,
            #     tunnelIP4Dst = int(accessIP),
            #     tunnelTEID = n3TEID + i,
            #     srcIP = int(startIP + i),
            #     srcIfaceMask = 0xFF,
            #     tunnelIP4DstMask = 0xFFFFFFFF,
            #     tunnelTEIDMask = 0xFFFFFFFF,
            #     srcIPMask = 0xFFFFFFFF,
            #     precedence = 255,
            #     fseID = n3TEID + i,
            #     ctrID = i,
            #     farID = self.n6,
            #     qerIDList = [self.n6, 1],
            #     needDecap = 1,
            # )

            self.addPDR(pdrDown)
            # self.addPDR(pdrUp)

            # create N6 UL and DL FARs for 
            farDown = self.createFAR(
                farID = i,
                fseID = n3TEID + i,
                applyAction = self.actionForward,
                dstIntf = self.dstAccess,
                tunnelType = 0x1,
                tunnelIP4Src = int(accessIP),
                tunnelIP4Dst = int(enbIP), # only one eNB to send to downlink
                tunnelTEID = 0,
                tunnelPort = tunnelGTPUPort,
            )

            # farUp = self.createFAR(
            #     farID = self.n9,
            #     fseID = n3TEID + i,
            #     applyAction = self.actionForward,
            #     dstIntf = self.dstCore,
            #     tunnelType = 0x1,
            #     tunnelIP4Src = int(coreIP),
            #     tunnelIP4Dst = int(AUPFIP), # uplink IP, upf to ?
            #     tunnelTEID = n9TEID + i,
            #     tunnelPort = tunnelGTPUPort,
            # )

            self.addFAR(farDown)
            # self.addFAR(farUp)

        # TODO: set up trex to send traffic thru UPF
        print("Setting up TRex client...")
        # vm = STLVM()
        # vm.var(
        #     name="dst",
        #     min_value=str(startIP),
        #     max_value=str(endIP),
        #     size=4,
        #     op="random",
        # )
        # vm.write(fv_name="dst", pkt_offset="IP.dst")
        # vm.fix_chksum()

        # pkt = testutils.simple_udp_packet(
        #     pktlen=1400,
        #     eth_dst=UPF_DEST_MAC,
        #     with_udp_chksum=False,
        # )
        # stream = STLStream(
        #     packet=STLPktBuilder(pkt=pkt, vm=vm),
        #     mode=STLTXCont(bps_L1=RATE),
        # )
        # self.trex_client.add_streams(stream, ports=[TREX_SENDER_PORT])

        # self.trex_client.start(
        #     ports=[TREX_SENDER_PORT], mult="1", duration=15
        # )
        # s_time = time.time()
        # print(f"End time was {time.time() - s_time()}")

        # TODO: pull metrics from BESS and verify results
        self.getPortStats(ifname="Access")

        return
