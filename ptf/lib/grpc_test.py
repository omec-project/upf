# Copyright 2021-present Open Networking Foundation
# SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

from collections import namedtuple
from functools import wraps
from pprint import pprint

from ptf.base_tests import BaseTest
import ptf.testutils as testutils
from google.protobuf.any_pb2 import Any
from google.protobuf.json_format import MessageToDict
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

        self.gateMeter = 0x0
        self.gateDrop = 0x5
        self.gateUnmeter = 0x6

        self.pdrs = []
        self.fars = []
        self.appQers = []
        self.sessionQers = []

        # activate grpc connection to bess
        bess_server_addr = testutils.test_param_get("bess_upf_addr")
        self.channel = grpc.insecure_channel(target=bess_server_addr)
        self.bess_client = pb.BESSControlStub(self.channel)
    
    """ API for getting metrics from BESS-UPF """

    def getPortStats(self, ifname):
        # to get bess interface names:
        # `docker exec -it bess ./bessctl`
        # `$ show port`
        req = bess_msg.GetPortStatsRequest(
            name = ifname + "Fast",
        )

        return self.bess_client.GetPortStats(req)
    
    def _readFlowMeasurement(self, module, clear, quantiles):
        # pack request for flow measurements and send to bess
        request = module_msg.FlowMeasureCommandReadArg(
            clear=clear,
            latency_percentiles=quantiles,
            jitter_percentiles=quantiles,
        )
        any = Any()
        any.Pack(request)

        response = self.bess_client.ModuleCommand(
            bess_msg.CommandRequest(
                name = module,
                cmd = "read",
                arg = any,
            ),
            timeout=5,
        )

        # unpack response and return results
        data = response.data
        msg = module_msg.FlowMeasureReadResponse()
        if data.Is(module_msg.FlowMeasureReadResponse.DESCRIPTOR):
            data.Unpack(msg)

        msg = MessageToDict(msg)
        if "statistics" in msg:
            return msg["statistics"]
        
        return msg

    def getSessionStats(self, q=[50, 90, 99], quiet=False):

        # Pre-Qos Measurement Module
        qosStatsInResp = self._readFlowMeasurement(
            module="preQosFlowMeasure",
            clear=True,
            quantiles=q,
        )
        if not quiet:
            print("Pre-QoS measurement module:")
            pprint(qosStatsInResp)
            print()

        # Post-Qos Downlink Measurement Module
        postDlQosStatsResp = self._readFlowMeasurement(
            module="postDLQosFlowMeasure",
            clear=True,
            quantiles=q,
        )
        if not quiet:
            print("Post-QoS downlink measurement module:")
            pprint(postDlQosStatsResp)
            print()

        # Post-Qos Uplink Measurement Module
        postUlQosStatsResp = self._readFlowMeasurement(
            module="postULQosFlowMeasure",
            clear=True,
            quantiles=q,
        )
        if not quiet:
            print("Post-QoS uplink measurement module:")
            pprint(postUlQosStatsResp)
            print()

        return {
            "preQos":    qosStatsInResp,
            "postDlQos": postDlQosStatsResp,
            "postUlQos": postUlQosStatsResp,
        }

    """ API for configuring BESS-UPF """

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
        gate=0,
        qerID=0,
        qfi=0,
        ulStatus=0,
        dlStatus=0,
        ulMbr=0,
        dlMbr=0,
        ulGbr=0,
        dlGbr=0,
        fseID=0,
        fseidIP=0,
        burstDurationMs=1000,
    ):
        fields = (
            'gate',
            'qerID',
            'qfi',
            'ulStatus',
            'dlStatus',
            'ulMbr',
            'dlMbr',
            'ulGbr',
            'dlGbr',
            'fseID',
            'fseidIP',
            'burstDurationMs',
        )
        defaults = [
            gate,
            qerID,
            qfi,
            ulStatus,
            dlStatus,
            ulMbr, # Kbps
            dlMbr, # Kbps
            ulGbr, # Kbps
            dlGbr, # Kbps
            fseID,
            fseidIP,
            burstDurationMs,
        ]
        QER = namedtuple('QER', fields, defaults=defaults)
        return QER()

    def addPDR(self, pdr, debug=False):
        for qerID in pdr.qerIDList:
            qerID = qerID
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
            ),
            timeout=5,
        )
        if debug:
            print(response)

        self.pdrs.append(pdr)

    def delPDR(self, pdr, debug=False):
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
            ),
            timeout=5,
        )
        if debug:
            print(response)
    
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

    def addFAR(self, far, debug=False):
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
            ),
            timeout=5,
        )
        if debug:
            print(response)

        self.fars.append(far)

    def delFAR(self, far, debug=False):
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
            ),
            timeout=5,
        )
        if debug:
            print(response)

    def _calcRates(self, ulGbr, ulMbr, dlGbr, dlMbr, burstDuration):
        # calculate uplink burst sizes
        ulCbs = (float(ulGbr) * 1000 / 8) * (burstDuration / 1000)
        ulPbs = (float(ulMbr) * 1000 / 8) * (burstDuration / 1000)
        ulEbs = ulPbs
        if ulMbr != 0 or ulGbr != 0:
            ulCir = max(ulGbr * 1000 / 8, 1)
            ulPir = max(ulMbr * 1000 / 8, ulCir)
        else:
            ulCir = 0
            ulPir = 0

        # calculate downlink burst sizes
        dlCbs = (float(dlGbr) * 1000 / 8) * (burstDuration / 1000)
        dlPbs = (float(dlMbr) * 1000 / 8) * (burstDuration / 1000)
        dlEbs = dlPbs
        if dlMbr != 0 or dlGbr != 0:
            dlCir = max(dlGbr * 1000 / 8, 1)
            dlPir = max(dlMbr * 1000 / 8, dlCir)
        else:
            dlCir = 0
            dlPir = 0

        fields = [
            'ulCbs', 'ulPbs', 'ulEbs', 'ulCir', 'ulPir',
            'dlCbs', 'dlPbs', 'dlEbs', 'dlCir', 'dlPir',
        ]
        defaults = [
            ulCbs, ulPbs, ulEbs, ulCir, ulPir, dlCbs, dlPbs, dlEbs, dlCir, dlPir,
        ]

        rates = namedtuple('rates', fields, defaults=defaults)
        return rates()

    def addApplicationQER(self, qer, debug=False):
        ''' installs uplink and downlink applicaiton QER '''
        rates = self._calcRates(
            qer.ulGbr,
            qer.ulMbr,
            qer.dlGbr,
            qer.dlMbr,
            qer.burstDurationMs,
        )

        # construct UL/DL QosCommandAddArg's and send to BESS
        for srcIface in [self.access, self.core]:
            f = module_msg.QosCommandAddArg(
                gate = qer.gate,
                cir = int(rates.ulCir) if srcIface == self.access else int(rates.dlCir),
                pir = int(rates.ulPir) if srcIface == self.access else int(rates.dlPir),
                cbs = int(rates.ulCbs) if srcIface == self.access else int(rates.dlCbs),
                pbs = int(rates.ulPbs) if srcIface == self.access else int(rates.dlPbs),
                ebs = int(rates.ulEbs) if srcIface == self.access else int(rates.dlEbs),
                fields = [
                    util_msg.FieldData(value_int = srcIface),
                    util_msg.FieldData(value_int = qer.qerID),
                    util_msg.FieldData(value_int = qer.fseID)
                ],
                values = [
                    util_msg.FieldData(value_int = qer.qfi)
                ],
            )

            any = Any()
            any.Pack(f)

            response = self.bess_client.ModuleCommand(
                bess_msg.CommandRequest(
                    name = "appQERLookup",
                    cmd = "add",
                    arg = any
                ),
                timeout=5,
            )
            if debug:
                print(response)
        
        self.appQers.append(qer)

    def delApplicationQER(self, qer, debug=False):
        ''' deletes uplink and downlink application QER '''
        for srcIface in [self.access, self.core]:
            f = module_msg.QosCommandDeleteArg(
                fields =  [
                    util_msg.FieldData(value_int = srcIface),
                    util_msg.FieldData(value_int = qer.qerID),
                    util_msg.FieldData(value_int = qer.fseID),
                ],
            )
            any = Any()
            any.Pack(f)

            response = self.bess_client.ModuleCommand(
                bess_msg.CommandRequest(
                    name = "appQERLookup",
                    cmd = "delete",
                    arg = any
                ),
                timeout=5,
            )
            if debug:
                print(response)

    def addSessionQER(self, qer, debug=False):
        ''' installs uplink and downlink session QER '''
        rates = self._calcRates(
            qer.ulGbr,
            qer.ulMbr,
            qer.dlGbr,
            qer.dlMbr,
            qer.burstDurationMs,
        )

        # construct UL/DL QosCommandAddArg's and send to BESS
        for srcIface in [self.access, self.core]:
            f = module_msg.QosCommandAddArg(
                gate = qer.gate,
                cir = int(rates.ulCir) if srcIface == self.access else int(rates.dlCir),
                pir = int(rates.ulPir) if srcIface == self.access else int(rates.dlPir),
                cbs = int(rates.ulCbs) if srcIface == self.access else int(rates.dlCbs),
                pbs = int(rates.ulPbs) if srcIface == self.access else int(rates.dlPbs),
                ebs = int(rates.ulEbs) if srcIface == self.access else int(rates.dlEbs),
                fields = [
                    util_msg.FieldData(value_int = srcIface),
                    util_msg.FieldData(value_int = qer.fseID)
                ],
            )

            any = Any()
            any.Pack(f)

            response = self.bess_client.ModuleCommand(
                bess_msg.CommandRequest(
                    name = "sessionQERLookup",
                    cmd = "add",
                    arg = any
                ),
                timeout=5,
            )
            if debug:
                print(response)
        
        self.sessionQers.append(qer)

    def delSessionQER(self, qer, debug=False):
        ''' deletes uplink and downlink session QER '''
        for srcIface in [self.access, self.core]:
            f = module_msg.QosCommandDeleteArg(
                fields =  [
                    util_msg.FieldData(value_int = srcIface),
                    util_msg.FieldData(value_int = qer.fseID),
                ],
            )
            any = Any()
            any.Pack(f)

            response = self.bess_client.ModuleCommand(
                bess_msg.CommandRequest(
                    name = "sessionQERLookup",
                    cmd = "delete",
                    arg = any
                ),
                timeout=5,
            )
            if debug:
                print(response)

    def tearDown(self):
        print("Closing gRPC channel...")
        self.channel.close()


""" Functionality for flow cleanup after tests """

def cleanupRules(test):
    for pdr in test.pdrs:
        test.delPDR(pdr)

    for far in test.fars:
        test.delFAR(far)

    for aQer in test.appQers:
        test.delApplicationQER(aQer)

    for sQer in test.sessionQers:
        test.delSessionQER(sQer)

    return

def autocleanup(f):
    @wraps(f)
    def handle(*args, **kwargs):
        test = args[0]
        assert isinstance(test, GrpcTest)

        try:
            # Clear QoS stats on BESS before test runs
            test.getSessionStats(quiet=True)

            return f(*args, **kwargs)

        finally:
            # cleanup rules for pdrs, fars, app qers and session qers
            cleanupRules(test)

            # clear lists
            test.pdrs = []
            test.fars = []
            test.appQers = []
            test.sessionQers = []

    return handle
