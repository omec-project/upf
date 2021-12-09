# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2021 Open Networking Foundation

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
        self.addPDR(pdr, debug=True)
        print()

        # Testing purposes: verify bess fails to find PDR when modified
        # pdr = pdr._replace(srcIfaceMask=0xAF)
        print("del pdr response:")
        self.delPDR(pdr, debug=True)
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
        self.addFAR(far, debug=True)
        print()

        # Testing purposes: verify bess fails to find FAR when modified
        # far = far._replace(fseID=0xA0000000)
        print("del far response:")
        self.delFAR(far, debug=True)
        print()
    
class QerAppTest(TrexTest, GrpcTest):
    def runTest(self):
        # configure as basic N6 UL/DL QER
        qer = self.createQER(
            gate = self.gateUnmeter,
            qfi = 9,
            qerID = self.n6,
            fseID = 0x30000000,
            ulGbr = 0,
            ulMbr = 0,
            dlGbr = 0,
            dlMbr = 0,
            burstDurationMs = 100,
        )

        print("add qer response:")
        self.addApplicationQER(qer, debug=True)
        print()

        print("del qer response:")
        self.delApplicationQER(qer, debug=True)
        print()

class QerSessionTest(TrexTest, GrpcTest):
    def runTest(self):
        # configure as basic N6 UL/DL QER
        qer = self.createQER(
            gate = self.gateUnmeter,
            qfi = 0,
            qerID = 1,
            fseID = 0x30000000,
            ulGbr = 0,
            ulMbr = 0,
            dlGbr = 0,
            dlMbr = 0,
            burstDurationMs = 100,
        )

        print("add qer response:")
        self.addSessionQER(qer, debug=True)
        print()

        print("del qer response:")
        self.delSessionQER(qer, debug=True)
        print()

class MetricsTest(GrpcTest):
    @autocleanup
    def runTest(self):
        print(self.getPortStats("core"))
        self.printSessionStats()
