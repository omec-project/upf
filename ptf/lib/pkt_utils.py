# SPDX-FileCopyrightText: Copyright 2022-present Open Networking Foundation.
# SPDX-License-Identifier: Apache-2.0

from scapy.contrib.gtp import GTP_U_Header, GTPPDUSessionContainer
from scapy.layers.inet import IP, UDP
from scapy.layers.l2 import Ether

GTPU_PORT = 2152


def pkt_add_gtpu(
    pkt,
    out_ipv4_src,
    out_ipv4_dst,
    teid,
    sport=GTPU_PORT,
    dport=GTPU_PORT,
    ext_psc_type=None,
    ext_psc_qfi=None,
):
    """
    Encapsulates the given pkt with GTPU tunnel headers.
    """
    gtp_pkt = (
        Ether(src=pkt[Ether].src, dst=pkt[Ether].dst)
        / IP(src=out_ipv4_src, dst=out_ipv4_dst, tos=0, id=0x1513, flags=0, frag=0,)
        / UDP(sport=sport, dport=dport, chksum=0)
        / GTP_U_Header(gtp_type=255, teid=teid)
    )
    if ext_psc_type is not None:
        # Add QoS Flow Identifier (QFI) as an extension header (required for 5G RAN)
        gtp_pkt = gtp_pkt / GTPPDUSessionContainer(type=ext_psc_type, QFI=ext_psc_qfi)
    return gtp_pkt / pkt[Ether].payload
