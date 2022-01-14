#!/usr/local/bin/python3

# SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
#
# SPDX-License-Identifier: LicenseRef-ONF-Member-1.0

import argparse
import struct
import time
import socket
from dataclasses import dataclass, field
from ipaddress import IPv4Network, IPv4Address, AddressValueError
from threading import Lock, Thread
from typing import Dict, Generator, Optional, IO, Union

import ifcfg
from scapy import all as scapy
from scapy.contrib import pfcp
from scapy.layers.inet import IP, UDP
from scapy.layers.l2 import Ether
from scapy.utils import wrpcap

MSG_TYPES = {name: num for num, name in pfcp.PFCPmessageType.items()}
CAUSE_TYPES = {name: num for num, name in pfcp.CauseValues.items()}
IE_TYPES = {name: num for num, name in pfcp.IEType.items()}
HEARTBEAT_PERIOD = 5  # in seconds
UDP_PORT_PFCP = 8805
IFACE_ACCESS = 0
IFACE_CORE = 1


@dataclass()
class UeFlow:
    teid: int
    pdr_id: int
    far_id: int
    qer_id: int
    urr_id: int


@dataclass()
class Session:
    our_seid: int
    ue_addr: IPv4Address
    peer_seid: Optional[int] = None
    uplink: UeFlow = None
    downlink: UeFlow = None
    disable_caching: bool = False
    sent_pdrs: Dict[int, pfcp.IE_Compound] = field(default_factory=dict)
    sent_fars: Dict[int, pfcp.IE_Compound] = field(default_factory=dict)
    sent_urrs: Dict[int, pfcp.IE_Compound] = field(default_factory=dict)
    sent_qers: Dict[int, pfcp.IE_Compound] = field(default_factory=dict)

    def clear_sent_rules(self):
        """
        Wipe out the cache of previously sent rules
        :return: None
        """
        self.sent_pdrs.clear()
        self.sent_fars.clear()
        self.sent_urrs.clear()
        self.sent_qers.clear()

    def is_created(self):
        return self.peer_seid is not None

    def set_peer_seid(self, seid: int):
        self.peer_seid = seid

    def add_to_req_if_rule_new(self, req: Union[pfcp.PFCPSessionEstablishmentRequest,
                                                pfcp.PFCPSessionModificationRequest],
                               rule: pfcp.IE_Compound, rule_id: int, rule_type: str):
        """
        Check if the soon-to-be-sent rule has been previously sent to the PFCP agent,
        and then optimistically store the rule as sent
        :param req: the request in which the rule will be added
        :param rule: the rule to add to the request if an identical rule wasn't already sent
        :param rule_id: the ID of the rule to be sent
        :param rule_type: the rule type as a string: 'pdr', 'far', 'urr', or 'qer'
        :return: None
        """
        if self.disable_caching:
            if verbosity > 0:
                print("Adding %s with ID %d to request for SEID %d" %
                      (rule_type, rule_id, self.our_seid))
            req.IE_list.append(rule)
            return
        rule_cache: dict
        rule_type_to_dict = {
            "pdr": self.sent_pdrs,
            "far": self.sent_fars,
            "urr": self.sent_urrs,
            "qer": self.sent_qers
        }
        rule_cache = rule_type_to_dict.get(rule_type, None)
        if rule_cache is None:
            raise Exception("Bad rule type passed to rule cacher")

        previous_rule = rule_cache.get(rule_id, None)
        rule_cache[rule_id] = rule
        if previous_rule is None or previous_rule != rule:
            if verbosity > 0:
                print("Adding %s with ID %d to request for SEID %d" %
                      (rule_type, rule_id, self.our_seid))
            req.IE_list.append(rule)
        else:
            if verbosity > 0:
                print("Not adding %s with ID %d, already sent for SEID %d" %
                      (rule_type, rule_id, self.our_seid))

    def add_rules_to_request(self, args: argparse.Namespace,
                             request: Union[pfcp.PFCPSessionEstablishmentRequest,
                                            pfcp.PFCPSessionModificationRequest], add_pdrs=True,
                             add_fars=True, add_urrs=True, add_qers=True):
        if add_pdrs:
            pdr_up = craft_pdr(session=self, flow=self.uplink, src_iface=IFACE_ACCESS,
                               from_tunnel=True, tunnel_dst=args.s1u_addr,
                               precedence=args.pdr_precedence)
            self.add_to_req_if_rule_new(request, pdr_up, self.uplink.pdr_id, "pdr")
            pdr_down = craft_pdr(session=self, flow=self.downlink, src_iface=IFACE_CORE,
                                 from_tunnel=False, precedence=args.pdr_precedence)
            self.add_to_req_if_rule_new(request, pdr_down, self.downlink.pdr_id, "pdr")

        if add_fars:
            far_up = craft_far(session=self, far_id=self.uplink.far_id, forward_flag=True,
                               dst_iface=IFACE_CORE, tunnel=False)
            self.add_to_req_if_rule_new(request, far_up, self.uplink.far_id, "far")
            # The downlink FAR should only tunnel if this is an update message. Our PFCP agent does not support
            #  outer header creation on session establishment, only session modification
            far_down = craft_far(session=self, far_id=self.downlink.far_id, forward_flag=True,
                                 dst_iface=IFACE_ACCESS, tunnel=self.is_created(),
                                 tunnel_dst=args.enb_addr, teid=self.downlink.teid,
                                 buffer_flag=args.buffer, notifycp_flag=args.notifycp)
            self.add_to_req_if_rule_new(request, far_down, self.downlink.far_id, "far")

        if add_qers:
            qer_up = craft_qer(session=self, qer_id=self.uplink.qer_id)
            self.add_to_req_if_rule_new(request, qer_up, self.uplink.qer_id, "qer")
            qer_down = craft_qer(session=self, qer_id=self.downlink.qer_id)
            self.add_to_req_if_rule_new(request, qer_down, self.downlink.qer_id, "qer")

        if add_urrs:
            urr_up = craft_urr(session=self, urr_id=self.uplink.urr_id, quota=100000,
                               threshold=40000)
            self.add_to_req_if_rule_new(request, urr_up, self.uplink.urr_id, "urr")
            urr_down = craft_urr(session=self, urr_id=self.downlink.urr_id, quota=100000,
                                 threshold=50000)
            self.add_to_req_if_rule_new(request, urr_down, self.downlink.urr_id, "urr")


# Global non-constants
our_addr: str = ""
peer_addr: str = ""
sequence_number: int = 0
thread_lock = Lock()
association_established = False
script_terminating = False
active_sessions: Dict[int, Session] = {}
pcap_filename: Optional[str] = None
verbosity: int = 0
# End global non-constants


def capture(pkt: scapy.Packet):
    """
    Write the given packet to the PCAP file that was defined when the mock SMF was launched.
    If no such file was given, do nothing.
    :param pkt: The packet to record in the PCAP file
    :return:
    """
    if Ether not in pkt:
        pkt = Ether() / pkt
    if pcap_filename:
        wrpcap(pcap_filename, pkt, append=True)


def get_sessions_from_args(args: argparse.Namespace, create_if_missing: bool = False):
    """
    Generate session objects from arguments passed along with the input command
    :param args: the input arguments
    :param create_if_missing: if no session object exists for a SEID specified in the input arguments,
    create it. Otherwise the SEID is skipped
    :return:
    """
    ue_addr_gen = get_addresses_from_prefix(args.ue_pool, args.session_count)
    seid_gen = iter(
        range(args.base if args.base else args.seid_base,
              (args.base if args.base else args.seid_base) + args.session_count))
    teid_gen = iter(
        range(args.base if args.base else args.teid_base,
              (args.base if args.base else args.teid_base) + args.session_count * 2))

    for _ in range(args.session_count):
        seid: int = next(seid_gen)
        if seid in active_sessions:
            yield active_sessions[seid]
        elif create_if_missing:
            active_sessions[seid] = Session(
                our_seid=seid, ue_addr=next(ue_addr_gen),
                uplink=UeFlow(teid=next(teid_gen), pdr_id=args.base if args.base else args.pdr_base,
                              far_id=args.base if args.base else args.far_base,
                              qer_id=args.base if args.base else args.qer_base,
                              urr_id=args.urr_base),
                downlink=UeFlow(teid=next(teid_gen),
                                pdr_id=(args.base if args.base else args.pdr_base) + 1,
                                far_id=(args.base if args.base else args.far_base) + 1,
                                qer_id=(args.base if args.base else args.qer_base) + 1,
                                urr_id=(args.base if args.base else args.urr_base) + 1))
            yield active_sessions[seid]
        else:
            print("WARNING: skipping invalid session with ID %d" % seid)
            continue


def get_addresses_from_prefix(prefix: IPv4Network,
                              count: int) -> Generator[IPv4Address, None, None]:
    """
    Generator for yielding Ip4Addresses from the provided prefix.
    :param prefix: the prefix from which addresses should be generated
    :param count: how many addresses to yield
    :return: an address generator
    """
    # Currently this doesn't allow the address with host bits all 0,
    #  so the first host address is (prefix_addr & mask) + 1
    if count >= 2**(prefix.max_prefixlen - prefix.prefixlen):
        raise Exception("trying to generate more addresses than a prefix contains!")
    base_addr = ip2int(prefix.network_address) + 1
    offset = 0
    while offset < count:
        yield IPv4Address(base_addr + offset)
        offset += 1


def ip2int(addr: IPv4Address):
    return struct.unpack("!I", addr.packed)[0]


def int2ip(addr: int):
    return IPv4Address(addr)


def get_sequence_num(reset=False):
    """
    Generate a sequence number for a PFCP message.
    :param reset: if true, resets the sequence number counter
    :return: a sequence number to be used in a PFCP message
    """
    thread_lock.acquire()
    global sequence_number
    if reset:
        sequence_number = 0
    sequence_number += 1
    thread_lock.release()
    return sequence_number


def craft_fseid(seid: int, address: str) -> pfcp.IE_Compound:
    fseid = pfcp.IE_FSEID()
    fseid.v4 = 1
    fseid.seid = seid
    fseid.ipv4 = address
    return fseid


def craft_pdr(session: Session, flow: UeFlow, src_iface: int, from_tunnel=False,
              tunnel_dst: str = None, precedence=2) -> pfcp.IE_Compound:
    pdr = pfcp.IE_UpdatePDR() if flow.pdr_id in session.sent_pdrs else pfcp.IE_CreatePDR()
    pdr_id = pfcp.IE_PDR_Id()
    pdr_id.id = flow.pdr_id
    pdr.IE_list.append(pdr_id)
    _precedence = pfcp.IE_Precedence()
    _precedence.precedence = precedence
    pdr.IE_list.append(_precedence)

    # Packet Detection Information
    pdi = pfcp.IE_PDI()

    # Source interface
    source_interface = pfcp.IE_SourceInterface()
    source_interface.interface = src_iface
    pdi.IE_list.append(source_interface)

    if from_tunnel:
        if tunnel_dst is None or flow.teid is None:
            raise Exception("ERROR: tunnel dst and teid should be provided for tunnel PDR")
        # Add the F-TEID to the PDI
        fteid = pfcp.IE_FTEID()
        fteid.V4 = 1
        fteid.TEID = flow.teid
        fteid.ipv4 = tunnel_dst
        pdi.IE_list.append(fteid)
        # Add outer header removal instruction to PDR
        outer_header_removal = pfcp.IE_OuterHeaderRemoval()
        outer_header_removal.header = 0
        pdr.IE_list.append(outer_header_removal)
    else:
        if session.ue_addr is None:
            raise Exception("UE address required for downlink PDRs!")
        # Add UE IPv4 address to the PDI
        _ue_addr = pfcp.IE_UE_IP_Address()
        _ue_addr.V4 = 1
        _ue_addr.ipv4 = session.ue_addr
        pdi.IE_list.append(_ue_addr)
        # If its not from a tunnel, then its from the internet
        net_instance = pfcp.IE_NetworkInstance()
        net_instance.instance = "internetinternetinternetinterne"
        pdi.IE_list.append(net_instance)

    # Add a fully wildcard SDF filter
    sdf = pfcp.IE_SDF_Filter()
    sdf.FD = 1
    sdf.flow_description = "0.0.0.0/0 0.0.0.0/0 0 : 65535 0 : 65535 0x0/0x0"
    pdi.IE_list.append(sdf)

    pdr.IE_list.append(pdi)

    # Add all rule IDs
    _far_id = pfcp.IE_FAR_Id()
    _far_id.id = flow.far_id
    _qer_id = pfcp.IE_QER_Id()
    _qer_id.id = flow.qer_id
    _urr_id = pfcp.IE_URR_Id()
    _urr_id.id = flow.urr_id
    pdr.IE_list.append(_far_id)
    pdr.IE_list.append(_qer_id)
    pdr.IE_list.append(_urr_id)

    return pdr


def craft_far(session: Session, far_id: int, forward_flag=False, drop_flag=False, buffer_flag=False,
              notifycp_flag=False, dst_iface: int = None, tunnel=False, tunnel_dst: str = None,
              teid: int = None) -> pfcp.IE_Compound:
    update = far_id in session.sent_fars
    far = pfcp.IE_UpdateFAR() if update else pfcp.IE_CreateFAR()
    _far_id = pfcp.IE_FAR_Id()
    _far_id.id = far_id
    far.IE_list.append(_far_id)

    # Apply Action
    apply_action = pfcp.IE_ApplyAction()
    apply_action.FORW = int(forward_flag)
    apply_action.DROP = int(drop_flag)
    apply_action.BUFF = int(buffer_flag)
    apply_action.NOCP = int(notifycp_flag)
    far.IE_list.append(apply_action)

    # Forwarding Parameters
    forward_param = pfcp.IE_ForwardingParameters(
    ) if not update else pfcp.IE_UpdateForwardingParameters()
    _dst_iface = pfcp.IE_DestinationInterface()
    _dst_iface.interface = dst_iface
    forward_param.IE_list.append(_dst_iface)

    if tunnel:
        if (not buffer_flag) and tunnel_dst is None or teid is None:
            raise Exception("ERROR: tunnel dst and teid should be provided for tunnel FAR")
        outer_header = pfcp.IE_OuterHeaderCreation()
        outer_header.GTPUUDPIPV4 = 1
        outer_header.ipv4 = tunnel_dst
        outer_header.TEID = teid if not buffer_flag else 0  # FARs that buffer have a TEID of zero
        forward_param.IE_list.append(outer_header)

    far.IE_list.append(forward_param)
    return far


def craft_qer(session: Session, qer_id: int, max_bitrate_up=12345678, max_bitrate_down=12345678,
              guaranteed_bitrate_up=12345678, guaranteed_bitrate_down=12345678) -> pfcp.IE_Compound:
    qer = pfcp.IE_UpdateQER() if qer_id in session.sent_qers else pfcp.IE_CreateQER()
    # QER ID
    _qer_id = pfcp.IE_QER_Id()
    _qer_id.id = qer_id
    qer.IE_list.append(_qer_id)
    # Gate Status
    gate1 = pfcp.IE_GateStatus()
    qer.IE_list.append(gate1)
    # Maximum Bitrate
    max_bitrate = pfcp.IE_MBR()
    max_bitrate.ul = max_bitrate_up
    max_bitrate.dl = max_bitrate_down
    qer.IE_list.append(max_bitrate)
    # Guaranteed Bitrate
    guaranteed_bitrate = pfcp.IE_GBR()
    guaranteed_bitrate.ul = guaranteed_bitrate_up
    guaranteed_bitrate.dl = guaranteed_bitrate_down
    qer.IE_list.append(guaranteed_bitrate)
    return qer


def craft_urr(session: Session, urr_id: int, quota: int, threshold: int) -> pfcp.IE_Compound:
    urr = pfcp.IE_UpdateURR() if urr_id in session.sent_urrs else pfcp.IE_CreateURR()
    # URR ID
    _urr_id = pfcp.IE_URR_Id()
    _urr_id.id = urr_id
    urr.IE_list.append(_urr_id)
    # Measurement Method
    measure_method = pfcp.IE_MeasurementMethod()
    measure_method.VOLUM = 1
    urr.IE_list.append(measure_method)
    # Report trigger
    report_trigger = pfcp.IE_ReportingTriggers()
    report_trigger.volume_threshold = 1
    report_trigger.volume_quota = 1
    urr.IE_list.append(report_trigger)
    # Volume quota
    volume_quota = pfcp.IE_VolumeQuota()
    volume_quota.TOVOL = 1
    volume_quota.total = quota
    urr.IE_list.append(volume_quota)
    # Volume threshold
    volume_threshold = pfcp.IE_VolumeThreshold()
    volume_threshold.TOVOL = 1
    volume_threshold.total = threshold
    urr.IE_list.append(volume_threshold)

    return urr


def craft_pfcp_association_setup_packet() -> scapy.Packet:
    # create PFCP packet
    pfcp_header = pfcp.PFCP()
    # create setup request packet
    setup_request = pfcp.PFCPAssociationSetupRequest()
    setup_request.version = 1
    # Let's add IEs into the message
    ie1 = pfcp.IE_NodeId()
    ie1.ipv4 = our_addr
    setup_request.IE_list.append(ie1)
    ie2 = pfcp.IE_RecoveryTimeStamp()
    setup_request.IE_list.append(ie2)
    return IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / setup_request


def craft_pfcp_association_release_packet() -> scapy.Packet:
    pfcp_header = pfcp.PFCP()
    # create release request packet
    release_request = pfcp.PFCPAssociationReleaseRequest()
    release_request.version = 1
    # Let's add IEs into the message
    ie1 = pfcp.IE_NodeId()
    ie1.ipv4 = our_addr
    release_request.IE_list.append(ie1)
    return IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / release_request


def craft_pfcp_session_est_packet(args: argparse.Namespace, session: Session) -> scapy.Packet:
    pfcp_header = pfcp.PFCP()
    pfcp_header.version = 1
    pfcp_header.S = 1
    pfcp_header.message_type = MSG_TYPES["session_establishment_request"]
    pfcp_header.seid = 0
    pfcp_header.seq = get_sequence_num()

    establishment_request = pfcp.PFCPSessionEstablishmentRequest()
    # add IEs into message
    nodeid = pfcp.IE_NodeId()
    nodeid.ipv4 = our_addr
    establishment_request.IE_list.append(nodeid)

    fseid = craft_fseid(session.our_seid, our_addr)
    establishment_request.IE_list.append(fseid)

    pdn_type = pfcp.IE_PDNType()
    establishment_request.IE_list.append(pdn_type)

    session.add_rules_to_request(args=args, request=establishment_request)

    return IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / establishment_request


def craft_pfcp_session_modify_packet(args: argparse.Namespace, session: Session) -> scapy.Packet:
    # fill pfcp header
    pfcp_header = pfcp.PFCP()
    pfcp_header.version = 1
    pfcp_header.S = 1
    pfcp_header.message_type = MSG_TYPES["session_modification_request"]
    if not session.is_created():
        raise Exception("Session %d has not yet been created, cannot modify" % session.our_seid)
    pfcp_header.seid = session.peer_seid
    pfcp_header.seq = get_sequence_num()

    modification_request = pfcp.PFCPSessionModificationRequest()
    fseid = craft_fseid(session.our_seid, our_addr)
    modification_request.IE_list.append(fseid)

    session.add_rules_to_request(args, modification_request, add_pdrs=False, add_urrs=False,
                                 add_qers=False)

    return IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / modification_request


def craft_pfcp_session_delete_packet(session: Session) -> scapy.Packet:
    pfcp_header = pfcp.PFCP()
    pfcp_header.version = 1
    pfcp_header.S = 1
    pfcp_header.message_type = MSG_TYPES["session_deletion_request"]
    if session.peer_seid is None:
        raise Exception("Peer SEID has not yet been received.")
    pfcp_header.seid = session.peer_seid
    pfcp_header.seq = get_sequence_num()

    deletion_request = pfcp.PFCPSessionDeletionRequest()
    fseid = craft_fseid(session.our_seid, our_addr)
    deletion_request.IE_list.append(fseid)

    delete_pkt = IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / deletion_request
    return delete_pkt


def send_recv_pfcp(pkt: scapy.Packet, expected_response_type: int, session: Optional[Session],
                   verbosity_override: int = verbosity) -> bool:
    """
    Send the given PFCP packet to the PFCP server, and wait for a response with the given PFCP message type.
    :param pkt: The packet to be sent to the server
    :param expected_response_type: The expected PFCP message type of the response
    :param session: If the message to be transmitted is associated with a session, this parameter will contain
    details about that session. For session-less messages (like association request and release), pass None.
    :param verbosity_override: Override for the script-wide verbosity
    :return: True if no errors are encountered, false otherwise
    """

    if verbosity_override > 1:
        pkt.show()
    capture(pkt)
    response = scapy.sr1(pkt, verbose=verbosity_override, timeout=5)
    if response is None:
        return False
    capture(response)
    if verbosity_override > 1:
        response.show()
    if response.message_type == expected_response_type:
        for ie in response.payload.IE_list:
            if ie.ie_type == IE_TYPES["Cause"]:
                if ie.cause not in [CAUSE_TYPES["Reserved"], CAUSE_TYPES["Request accepted"]]:
                    response_type_str = pfcp.PFCPmessageType[response.message_type]
                    print("ERROR in PFCP message of type %s: %s" %
                          (response_type_str, pfcp.CauseValues[ie.cause]))
                    return False
            elif ie.ie_type == IE_TYPES["F-SEID"]:
                if session is None:
                    raise Exception(
                        "Received PFCP response with session ID that we have no Session object to save to!"
                    )
                session.set_peer_seid(int(ie.seid))
    else:
        print("ERROR: Expected response of type %s but received %s" %
              (pfcp.PFCPmessageType[expected_response_type],
               pfcp.PFCPmessageType[response.message_type]))
        return False
    return True


def setup_pfcp_association(args: argparse.Namespace) -> None:
    get_sequence_num(reset=True)  # zero out the sequence number
    global association_established
    pkt = craft_pfcp_association_setup_packet()
    send_recv_pfcp(pkt, MSG_TYPES["association_setup_response"], session=None)
    association_established = True  # signal the heartbeat thread to start sending


def teardown_pfcp_association(args: argparse.Namespace) -> None:
    global association_established
    pkt = craft_pfcp_association_release_packet()
    send_recv_pfcp(pkt, MSG_TYPES["association_release_response"], session=None)
    association_established = False
    active_sessions.clear()


def create_pfcp_sessions(args: argparse.Namespace) -> None:
    for session in get_sessions_from_args(args, create_if_missing=True):
        if verbosity > 0:
            print("Creating session with SEID %d" % session.our_seid)
        pkt = craft_pfcp_session_est_packet(args, session)
        send_recv_pfcp(pkt, MSG_TYPES["session_establishment_response"], session)
        time.sleep(args.sleep_time)  # sleep before the next session creation


def modify_pfcp_sessions(args: argparse.Namespace) -> None:
    for session in get_sessions_from_args(args, create_if_missing=False):
        if verbosity > 0:
            print("Modifying session with SEID %d" % session.our_seid)
        pkt = craft_pfcp_session_modify_packet(args, session)
        send_recv_pfcp(pkt, MSG_TYPES["session_modification_response"], session)
        time.sleep(args.sleep_time)  # sleep before the next session modification


def delete_pfcp_sessions(args: argparse.Namespace) -> None:
    for session in get_sessions_from_args(args, create_if_missing=False):
        if verbosity > 0:
            print("Deleting session with SEID %d" % session.our_seid)
        pkt = craft_pfcp_session_delete_packet(session)
        send_recv_pfcp(pkt, MSG_TYPES["session_deletion_response"], session)
        del active_sessions[session.our_seid]
        time.sleep(args.sleep_time)  # sleep before the next session deletion


def send_pfcp_heartbeats() -> None:
    while True:
        for _ in range(HEARTBEAT_PERIOD):
            # semi-busy wait
            time.sleep(1)
            if script_terminating:
                return
        if not association_established:
            # Don't heartbeat unless an association is currently established
            continue
        pfcp_header = pfcp.PFCP()
        pfcp_header.version = 1
        pfcp_header.S = 0  # SEID flag false
        pfcp_header.seq = get_sequence_num()
        pfcp_header.message_type = MSG_TYPES["heartbeat_request"]

        heartbeat = pfcp.PFCPHeartbeatRequest()
        heartbeat.version = 1
        heartbeat.IE_list.append(pfcp.IE_RecoveryTimeStamp())

        pkt = IP(src=our_addr, dst=peer_addr) / UDP() / pfcp_header / heartbeat
        send_recv_pfcp(pkt, MSG_TYPES["heartbeat_response"], session=None, verbosity_override=0)


class ArgumentParser(argparse.ArgumentParser):

    def error(self, message):
        # This override stops the argument parser from calling exit() on error
        raise Exception("Bad parser input: %s" % message)


def interrupt_association(args: argparse.Namespace) -> None:
    global association_established
    association_established = False  # stop the heartbeat thread
    active_sessions.clear()  # kill all the active sessions


def terminate(args: argparse.Namespace) -> None:
    global script_terminating
    if association_established:
        print("Exiting before association deleted. Deleting..")
        delete_pfcp_sessions(args)
    script_terminating = True
    exit()


def handle_user_input(input_file: Optional[IO] = None, output_file: Optional[IO] = None) -> None:
    global script_terminating

    user_choices = {
        "associate": ("Setup PFCP Association", setup_pfcp_association),
        "create": ("Create PFCP Session(s)", create_pfcp_sessions),
        "modify": ("Modify PFCP Session(s)", modify_pfcp_sessions),
        "delete": ("Delete PFCP Session(s)", delete_pfcp_sessions),
        "disassociate": ("Teardown PFCP Association", teardown_pfcp_association),
        "interrupt": ("Ungracefully teardown PFCP association", interrupt_association),
        "stop": ("Exit script", terminate)
    }

    parser = ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument("choice", type=str, help="The PFCP client operation to perform",
                        choices=user_choices.keys())
    parser.add_argument("--session-count", type=int, default=1,
                        help="The number of sessions for which UE flows should be created.")
    parser.add_argument(
        "--sleep-time", type=float, default=0.0,
        help="How much time to sleep between sending PFCP requests for multiple sessions")
    parser.add_argument(
        "--buffer", action='store_true',
        help="If this argument is present, downlink FARs will have the buffering flag set to true")
    parser.add_argument(
        "--notifycp", action='store_true',
        help="If this argument is present, downlink FARs will have the notify CP flag set to true")
    parser.add_argument("--ue-pool", type=IPv4Network, default=IPv4Network("17.0.0.0/24"),
                        help="The IPv4 prefix from which UE addresses will be drawn.")
    parser.add_argument("--s1u-addr", type=IPv4Address, default=IPv4Address("140.0.0.1"),
                        help="The IPv4 address of the UPF's S1U interface")
    parser.add_argument("--enb-addr", type=IPv4Address, default=IPv4Address("140.0.100.1"),
                        help="The IPv4 address of the eNodeB")
    parser.add_argument(
        "--seid-base", type=int, default=1,
        help="The first SEID to use for the first UE session. " +
        "Further SEIDs will be generated by incrementing.")
    parser.add_argument(
        "--teid-base", type=int, default=255, help="The first TEID to use for the first UE flow. " +
        "Further TEIDs will be generated by incrementing.")
    parser.add_argument(
        "--pdr-base", type=int, default=1,
        help="The first PDR ID to use for the first UE of each session. " +
        "Further PDR IDs will be generated by incrementing.")
    parser.add_argument(
        "--far-base", type=int, default=1,
        help="The first FAR ID to use for the first UE of each session. " +
        "Further FAR IDs will be generated by incrementing.")
    parser.add_argument(
        "--urr-base", type=int, default=1,
        help="The first URR ID to use for the first UE of each session. " +
        "Further URR IDs will be generated by incrementing.")
    parser.add_argument(
        "--qer-base", type=int, default=1,
        help="The first QER ID to use for the first UE of each session. " +
        "Further QER IDs will be generated by incrementing.")
    parser.add_argument(
        "--base", type=int, default=None, help="First ID used to generate all other ID fields." +
        "If specified, overrides all the other --*-base arguments")
    parser.add_argument("--pdr-precedence", type=int, default=2,
                        help="The priority/precedence of PDRs.")

    def get_user_input():
        if not input_file:
            return input("Enter your selection : ")
        else:
            read_head = input_file.tell()
            line = input_file.readline()
            if line:
                print("returning %s" % line)
                return line
            time.sleep(0.5)
            input_file.seek(read_head)

    while True:
        print("=" * 40)
        for choice, (action_desc, action) in user_choices.items():
            print("\"%s\" - %s" % (choice, action_desc))
        try:
            args = parser.parse_args(get_user_input().split())
        except Exception as e:
            print(e)
            parser.print_help()
            continue
        try:
            choice_desc, choice_func = user_choices[args.choice]
            print("Selected %s" % choice_desc)
            choice_func(args)
        except Exception as e:
            # Catch the exception just long enough to signal the heartbeat thread to end
            script_terminating = True
            raise e


def main():
    global our_addr, peer_addr, pcap_filename

    our_addr = ifcfg.interfaces()['eth0']['inet']

    parser = argparse.ArgumentParser()
    parser.add_argument("upfaddr", help="Address or hostname of the UPF")
    parser.add_argument("--input-file", help="File to poll for input commands. Default is stdin")
    parser.add_argument("--output-file", help="File in which to write output. Default is stdout")
    parser.add_argument(
        "--pcap-file",
        help="File in which to write sent/received PFCP packets. Default is no capture")
    parser.add_argument('--verbose', '-v', action='count', default=0)
    args = parser.parse_args()
    input_file: Optional[IO] = None
    output_file: Optional[IO] = None
    # Try opening the files right now so we don't wait until the main loop to fail
    if args.input_file:
        input_file = open(args.input_file, "r")
    if args.output_file:
        output_file = open(args.output_file, "w")
    if args.pcap_file:
        # clear the pcap file
        pcap_filename = args.pcap_file
        open(pcap_filename, 'w').close()
    global verbosity
    verbosity = args.verbose

    try:
        peer_addr = socket.gethostbyname(args.upfaddr)
    except socket.gaierror as e:
        try:
            peer_addr = str(IPv4Address(args.upfaddr))
        except AddressValueError as e:
            print("Argument must be a valid hostname or address")
            exit(1)

    thread1 = Thread(target=handle_user_input, args=(input_file, output_file))
    thread2 = Thread(target=send_pfcp_heartbeats)

    thread1.start()
    thread2.start()
    thread1.join()
    thread2.join()


if __name__ == "__main__":
    main()
