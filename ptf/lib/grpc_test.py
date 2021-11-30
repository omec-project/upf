# Copyright 2021-present Open Networking Foundation
# SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

from collections import namedtuple
from ptf.base_tests import BaseTest
import ptf.testutils as testutils
from google.protobuf.any_pb2 import Any
import grpc

import service_pb2_grpc as pb
import bess_msg_pb2 as bess_msg
import module_msg_pb2 as module_msg
import util_msg_pb2 as util_msg

class GrpcTest(BaseTest):
    def setUp(self):
        # initialize useful variables (for readability)
        self.access = 0x1
        self.core = 0x2
        self.dstAccess = self.access - 1
        self.dstCore = self.core - 1

        self.n3 = 0x0
        self.n6 = 0x1
        self.n9 = 0x2

        self.actionDrop = 0x1
        self.actionForward = 0x2
        self.actionBuffer = 0x4
        self.actionNotify = 0x8

        self.gateMeter = 0x4
        self.gateDrop = 0x5
        self.gateUnmeter = 0x6

        # activate grpc connection to bess
        bess_server_addr = testutils.test_param_get("bess_upf_addr")
        self.channel = grpc.insecure_channel(target=bess_server_addr)
        self.bess_client = pb.BESSControlStub(self.channel)
    
    def getPortStats(self, ifname):
        req = bess_msg.GetPortStatsRequest(
            name = ifname + "Fast",
        )

        print(self.bess_client.GetPortStats(req))

    def createPDR(
        self,
        srcIface=0,
        tunnelIP4Dst=0,
        tunnelTEID=0,
        srcIP=0,
        dstIP=0,
        srcPort=0,
        dstPort=0,
        proto=0,
        srcIfaceMask=0,
        tunnelIP4DstMask=0,
        tunnelTEIDMask=0,
        srcIPMask=0,
        dstIPMask=0,
        srcPortMask=0,
        dstPortMask=0,
        protoMask=0,
        precedence=0,
        pdrID=0,
        fseID=0,
        fseidIP=0,
        ctrID=0,
        farID=0,
        qerIDList=[],
        needDecap=0,
        allocIPFlag=False,
    ):

        fields = (
            'srcIface',
            'tunnelIP4Dst',
            'tunnelTEID',
            'srcIP',
            'dstIP',
            'srcPort',
            'dstPort',
            'proto',

            'srcIfaceMask',
            'tunnelIP4DstMask',
            'tunnelTEIDMask',
            'srcIPMask',
            'dstIPMask',
            'srcPortMask',
            'dstPortMask',
            'protoMask',

            'precedence',
            'pdrID',
            'fseID',
            'fseidIP',
            'ctrID',
            'farID',
            'qerIDList',
            'needDecap',
            'allocIPFlag',
        )
        defaults = [
            srcIface,
            tunnelIP4Dst,
            tunnelTEID,
            srcIP,
            dstIP,
            srcPort,
            dstPort,
            proto,

            srcIfaceMask,
            tunnelIP4DstMask,
            tunnelTEIDMask,
            srcIPMask,
            dstIPMask,
            srcPortMask,
            dstPortMask,
            protoMask,

            precedence,
            pdrID,
            fseID,
            fseidIP,
            ctrID,
            farID,
            qerIDList,
            needDecap,
            allocIPFlag,
        ]

        PDR =  namedtuple('PDR', fields, defaults=defaults)
        return PDR()

    def createFAR(
        self,
        farID=0,
        fseID=0,
        fseidIP=0,
        dstIntf=0,
        sendEndMarker=False,
        applyAction=0,
        tunnelType=0,
        tunnelIP4Src=0,
        tunnelIP4Dst=0,
        tunnelTEID=0,
        tunnelPort=0,
    ):
        fields = (
            'farID',
            'fseID',
            'fseidIP',

            'dstIntf',
            'sendEndMarker',
            'applyAction',
            'tunnelType',
            'tunnelIP4Src',
            'tunnelIP4Dst',
            'tunnelTEID',
            'tunnelPort',
        )
        defaults = [
            farID,
            fseID,
            fseidIP,

            dstIntf,
            sendEndMarker,
            applyAction,
            tunnelType,
            tunnelIP4Src,
            tunnelIP4Dst,
            tunnelTEID,
            tunnelPort,
        ]

        FAR = namedtuple('FAR', fields, defaults=defaults)
        return FAR()

    def createQER(
        self,
        qerID=0,
        qosLevel=0,
        qfi=0,
        ulStatus=0,
        dlStatus=0,
        ulMbr=0,
        dlMbr=0,
        ulGbr=0,
        dlGbr=0,
        fseID=0,
        fseidIP=0,
    ):
        fields = (
            'qerID',
            'qosLevel',
            'qfi',
            'ulStatus',
            'dlStatus',
            'ulMbr',
            'dlMbr',
            'ulGbr',
            'dlGbr',
            'fseID',
            'fseidIP',
        )
        defaults = [
            qerID,
            qosLevel,
            qfi,
            ulStatus,
            dlStatus,
            ulMbr, # Kbps
            dlMbr, # Kbps
            ulGbr, # Kbps
            dlGbr, # Kbps
            fseID,
            fseidIP,
        ]
        QER = namedtuple('QER', fields, defaults=defaults)
        return QER()

    def addPDR(self, pdr):
        for qer in pdr.qerIDList:
            qerID = qer
            break

        # parse params of pdr into WildcardMatchCommandAddArg
        f = module_msg.WildcardMatchCommandAddArg(
            gate = pdr.needDecap,
            priority = 4294967295 - pdr.precedence, # XXX: golang max 32 bit uint
            values = [
				util_msg.FieldData(value_int = pdr.srcIface),
				util_msg.FieldData(value_int = pdr.tunnelIP4Dst),
				util_msg.FieldData(value_int = pdr.tunnelTEID),
				util_msg.FieldData(value_int = pdr.srcIP),
				util_msg.FieldData(value_int = pdr.dstIP),
				util_msg.FieldData(value_int = pdr.srcPort),
				util_msg.FieldData(value_int = pdr.dstPort),
				util_msg.FieldData(value_int = pdr.proto),
            ],
            masks = [
				util_msg.FieldData(value_int = pdr.srcIfaceMask),
				util_msg.FieldData(value_int = pdr.tunnelIP4DstMask),
				util_msg.FieldData(value_int = pdr.tunnelTEIDMask),
				util_msg.FieldData(value_int = pdr.srcIPMask),
				util_msg.FieldData(value_int = pdr.dstIPMask),
				util_msg.FieldData(value_int = pdr.srcPortMask),
				util_msg.FieldData(value_int = pdr.dstPortMask),
				util_msg.FieldData(value_int = pdr.protoMask),
            ],
            valuesv = [
				util_msg.FieldData(value_int = pdr.pdrID),
				util_msg.FieldData(value_int = pdr.fseID),
				util_msg.FieldData(value_int = pdr.ctrID),
				util_msg.FieldData(value_int = qerID),
				util_msg.FieldData(value_int = pdr.farID),
            ],
        )

        # store into Any() message protobuf type
        any = Any()
        any.Pack(f)

        # send client module command with method add and arg stored Any()
        response = self.bess_client.ModuleCommand(
            bess_msg.CommandRequest(
                name = "pdrLookup",
                cmd = "add",
                arg = any
            )
        )

        return response

    def delPDR(self, pdr):
        # parse params of pdr into WildcardMatchCommandDeleteArg
        f = module_msg.WildcardMatchCommandDeleteArg(
            values = [
				util_msg.FieldData(value_int = pdr.srcIface),
				util_msg.FieldData(value_int = pdr.tunnelIP4Dst),
				util_msg.FieldData(value_int = pdr.tunnelTEID),
				util_msg.FieldData(value_int = pdr.srcIP),
				util_msg.FieldData(value_int = pdr.dstIP),
				util_msg.FieldData(value_int = pdr.srcPort),
				util_msg.FieldData(value_int = pdr.dstPort),
				util_msg.FieldData(value_int = pdr.proto),
            ],
            masks = [
				util_msg.FieldData(value_int = pdr.srcIfaceMask),
				util_msg.FieldData(value_int = pdr.tunnelIP4DstMask),
				util_msg.FieldData(value_int = pdr.tunnelTEIDMask),
				util_msg.FieldData(value_int = pdr.srcIPMask),
				util_msg.FieldData(value_int = pdr.dstIPMask),
				util_msg.FieldData(value_int = pdr.srcPortMask),
				util_msg.FieldData(value_int = pdr.dstPortMask),
				util_msg.FieldData(value_int = pdr.protoMask),
            ],
        )

        # store into Any() message protobuf type
        any = Any()
        any.Pack(f)

        # send client module command with method delete and arg stored Any()
        response = self.bess_client.ModuleCommand(
            bess_msg.CommandRequest(
                name = "pdrLookup",
                cmd = "delete",
                arg = any
            )
        )

        return response
    
    def _setActionValue(self, far):
        farForwardD = 0x0
        farForwardU = 0x1
        farDrop = 0x2
        farNotify = 0x3

        if (far.applyAction & self.actionForward) != 0:
            if far.dstIntf == self.dstAccess:
                return farForwardD
            elif far.dstIntf == self.dstCore:
                return farForwardU
        elif (far.applyAction & self.actionDrop) != 0: 
            return farDrop
        elif (far.applyAction & self.actionBuffer) != 0 : 
            return farNotify
        elif (far.applyAction & self.actionNotify) != 0: 
            return farNotify

    def addFAR(self, far):
        # set action value for far action
        action = self._setActionValue(far)

        # parse fields of far into ExactMatchCommandAddArg
        f = module_msg.ExactMatchCommandAddArg(
            gate = far.tunnelType,
            fields = [
                util_msg.FieldData(value_int = far.farID),
                util_msg.FieldData(value_int = far.fseID),
            ],
            values = [
				util_msg.FieldData(value_int = action),
				util_msg.FieldData(value_int = far.tunnelType),
				util_msg.FieldData(value_int = far.tunnelIP4Src),
				util_msg.FieldData(value_int = far.tunnelIP4Dst),
				util_msg.FieldData(value_int = far.tunnelTEID),
				util_msg.FieldData(value_int = far.tunnelPort),
            ],
        )

        # store into Any() message protobuf type
        any = Any()
        any.Pack(f)

        # send client module command with method add and arg stored Any()
        response = self.bess_client.ModuleCommand(
            bess_msg.CommandRequest(
                name = "farLookup",
                cmd = "add",
                arg = any
            )
        )

        return response

    def delFAR(self, far):
        # parse params of far into ExactMatchCommandDeleteArg
        f = module_msg.ExactMatchCommandDeleteArg(
            fields = [
				util_msg.FieldData(value_int = far.farID),
				util_msg.FieldData(value_int = far.fseID),
            ],
        )

        # store into Any() message protobuf type
        any = Any()
        any.Pack(f)

        # send client module command with method delete and arg stored Any()
        response = self.bess_client.ModuleCommand(
            bess_msg.CommandRequest(
                name = "farLookup",
                cmd = "delete",
                arg = any
            )
        )

        return response

    def addSessionQER(self, qer):
        # gate, (cir, pir, cbs, pbs, ebs), srciface, fseID
        return

    def delSessionQER(self, qer):
        # gate, (cir, pir, cbs, pbs, ebs), srciface, fseID
        return

    def addApplicationQER(self, qer):
        return
        # ''' adds QER for uplink and downlink application traffic '''
        # calculate burst sizes from ulGbr and ulMbr
        # ulCbs = (float(qer["ulGbr"]) * 1000 / 8) * (qer["burstDurationMs"] / 1000)
        # ulPbs = (float(qer["ulMbr"]) * 1000 / 8) * (qer["burstDurationMs"] / 1000)
        # ulEbs = ulPbs
        # ulCir = max(qer["ulGbr"] * 1000 / 8, 1)
        # ulPir = max(qer["ulMbr"] * 1000 / 8, ulCir)
        # calculate burst sizes from dlGbr and dlMbr
        # dlCbs = (float(qer["dlGbr"]) * 1000 / 8) * (qer["burstDurationMs"] / 1000)
        # dlPbs = (float(qer["dlMbr"]) * 1000 / 8) * (qer["burstDurationMs"] / 1000)
        # dlEbs = dlPbs
        # dlCir = max(qer["dlGbr"] * 1000 / 8, 1)
        # dlPir = max(qer["dlMbr"] * 1000 / 8, dlCir)
        # construct uplink QosCommandAddArg and send to BESS
        # for srcIface in [self.access, self.core]:
            # f = module_msg.QosCommandAddArg(
                # gate = qer["gate"],
                # cir = (int(ulCir) if srcIface == self.access else int(dlCir)),
                # pir = (int(ulPir) if srcIface == self.access else int(dlPir)),
                # cbs = (int(ulCbs) if srcIface == self.access else int(dlCbs)),
                # pbs = (int(ulPbs) if srcIface == self.access else int(dlPbs)),
                # ebs = (int(ulEbs) if srcIface == self.access else int(dlEbs)),
                # fields = [
                    # util_msg.FieldData(value_int = srcIface),
                    # util_msg.FieldData(value_int = qer["qerID"]),
                    # util_msg.FieldData(value_int = qer["fseID"])
                # ],
                # values = [
                    # util_msg.FieldData(value_int = qer["qfi"])
                # ],
            # )
            # any = Any()
            # any.Pack(f)
            # response = self.bess_client.ModuleCommand(
                # bess_msg.CommandRequest(
                    # name = "appQERLookup",
                    # cmd = "add",
                    # arg = any
                # )
            # )
            # print(response)

    def delApplicationQER(self, qer):
        return
    #     ''' deletes QER for uplink and downlink application traffic '''
    #     for srcIface in [self.access, self.core]:
    #         f = module_msg.QosCommandDeleteArg(
    #             fields =  [
    #                 util_msg.FieldData(value_int = srcIface),
    #                 util_msg.FieldData(value_int = qer["qerID"]),
    #                 util_msg.FieldData(value_int = qer["fseID"]),
    #             ]
    #         )
    #         any = Any()
    #         any.Pack(f)

    #         response = self.bess_client.ModuleCommand(
    #             bess_msg.CommandRequest(
    #                 name = "appQERLookup",
    #                 cmd = "add",
    #                 arg = any
    #             )
    #         )
    #         print(response)

    def tearDown(self):
        self.channel.close()
