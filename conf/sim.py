#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

from scapy.all import *
from scapy.contrib.gtp import *

# for ip2long
from conf.utils import *

# ====================================================
#       SIM Create Packet Functions
# ====================================================


def gen_inet_packet(size, src_mac, dst_mac, src_ip, dst_ip):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=10001, dport=10002)
    header = eth / ip / udp
    header_len = len(header)
    if size < header_len:
        raise ValueError(f"size {size} is smaller than header length {header_len}")
    payload_len = size - header_len
    pattern = "hello" + "0123456789" * 200
    payload = (pattern * (payload_len // len(pattern) + 1))[:payload_len]
    pkt = header / payload
    return bytes(pkt)


def gen_inet_sequpdate_args(max_session, start_ue_ip):
    kwargs = {
        "fields": [
            {
                "offset": 30,
                "size": 4,
                "min": ip2long(start_ue_ip),
                "max": ip2long(start_ue_ip) + max_session - 1,
            }
        ]
    }
    return kwargs


def gen_gtpu_packet(
    size,
    src_mac,
    dst_mac,
    src_ip,
    dst_ip,
    inner_src_ip,
    inner_dst_ip,
    teid,
    pdutype=None,
    qfi=None,
):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=2152, dport=2152)
    gtp = GTP_U_Header(teid=teid)
    inet_p = IP(src=inner_src_ip, dst=inner_dst_ip) / UDP(sport=10001, dport=10002)
    if pdutype is not None or qfi is not None:
        psc = GTPPDUSessionContainer(type=pdutype, QFI=qfi)
        header = eth / ip / udp / gtp / psc / inet_p
    else:
        header = eth / ip / udp / gtp / inet_p
    # Size the payload against the full packet header (outer eth/ip/udp +
    # GTP-U + optional PDU session container + inner IP/UDP), so the final
    # packet matches the requested `size`.
    header_len = len(header)
    if size < header_len:
        raise ValueError(f"size {size} is smaller than header length {header_len}")
    payload_len = size - header_len
    pattern = "hello" + "0123456789" * 200
    payload = (pattern * (payload_len // len(pattern) + 1))[:payload_len]
    pkt = header / payload
    return bytes(pkt)


def gen_gtpu_sequpdate_args(max_session, start_ue_ip, ue_ip_offset, start_teid):
    kwargs = {
        "fields": [
            {
                "offset": 46,
                "size": 4,
                "min": start_teid,
                "max": start_teid + max_session - 1,
            },
            {
                "offset": ue_ip_offset,
                "size": 4,
                "min": ip2long(start_ue_ip),
                "max": ip2long(start_ue_ip) + max_session - 1,
            },
        ]
    }
    return kwargs
