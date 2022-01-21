#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

from scapy.contrib.gtp import *
from scapy.all import *
# for ip2long
from conf.utils import *

# ====================================================
#       SIM Create Packet Functions
# ====================================================


def gen_inet_packet(size, src_mac, dst_mac, src_ip, dst_ip):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=10001, dport=10002)
    payload = ('hello' + '0123456789' * 200)[:size-len(eth/ip/udp)]
    pkt = eth/ip/udp/payload
    return bytes(pkt)


def gen_inet_sequpdate_args(max_session, start_ue_ip):
    kwargs = {"fields": [
        {'offset': 30, 'size': 4, 'min': ip2long(start_ue_ip),
         'max': ip2long(start_ue_ip)+max_session-1}]}
    return kwargs


def gen_gtpu_packet(size, src_mac, dst_mac, src_ip, dst_ip, inner_src_ip, inner_dst_ip, teid, pdutype=None, qfi=None):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=2152, dport=2152)
    inet_p = IP(src=inner_src_ip, dst=inner_dst_ip) / \
        UDP(sport=10001, dport=10002)
    payload = ('hello' + '0123456789' * 200)[:size-len(eth/ip/udp/inet_p)]
    if pdutype is not None or qfi is not None:
        psc = GTPPDUSessionContainer(type=pdutype, QFI=qfi)
        pkt = eth/ip/udp/GTP_U_Header(teid=teid)/psc/inet_p/payload
    else:
        pkt = eth/ip/udp/GTP_U_Header(teid=teid)/inet_p/payload
    return bytes(pkt)


def gen_ue_ntf_packet(size, src_mac, dst_mac, src_ip, dst_ip, inner_src_ip, inner_dst_ip, teid):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=2152, dport=2152)
    inet_p = IP(src=inner_src_ip, dst=inner_dst_ip) / \
        UDP(sport=10001, dport=10002)

    # Since scapy doesn't have a helper for STUN, the following blob contains
    # this STUN packet:
    #
    # Session Traversal Utilities for NAT
    #     Message Type: 0x0001 (Binding Request)
    #         .... ...0 ...0 .... = Message Class: 0x00 Request (0)
    #         ..00 000. 000. 0001 = Message Method: 0x0001 Binding (0x001)
    #         ..0. .... .... .... = Message Method Assignment: IETF Review (0x0)
    #     Message Length: 296
    #     Message Cookie: 2112a442
    #     Message Transaction ID: 3566674b4274783565347871
    #     Attributes
    #         Unknown
    #             Attribute Type: Unknown (0x8030)
    #             Attribute Length: 206
    #             Value: 0000b00f65794a68624763694f694a6b615849694c434a6c...
    #             Padding: 2
    #         USERNAME: dkvc21eeecviv3:cPtQ
    #             Attribute Type: USERNAME (0x0006)
    #             Attribute Length: 19
    #             Username: dkvc21eeecviv3:cPtQ
    #             Padding: 1
    #         Unknown
    #             Attribute Type: Unknown (0xc057)
    #             Attribute Length: 4
    #             Value: 0003000a
    #         ICE-CONTROLLED
    #             Attribute Type: ICE-CONTROLLED (0x8029)
    #             Attribute Length: 8
    #             Tie breaker: 6dd28d3d5768520a
    #         PRIORITY
    #             Attribute Type: PRIORITY (0x0024)
    #             Attribute Length: 4
    #             Priority: 1853693695
    #         MESSAGE-INTEGRITY
    #             Attribute Type: MESSAGE-INTEGRITY (0x0008)
    #             Attribute Length: 20
    #             HMAC-SHA1: 8c6403d231f11d99b0564a2c5e3e421e9b194c8b
    #         FINGERPRINT
    #             Attribute Type: FINGERPRINT (0x8028)
    #             Attribute Length: 4
    #             CRC-32: 0x29e46f38

    payload = \
        b"\x00\x01\x01\x28\x21\x12\xa4\x42\x35\x66\x67\x4b\x42\x74\x78\x35" \
        b"\x65\x34\x78\x71\x80\x30\x00\xce\x00\x00\xb0\x0f\x65\x79\x4a\x68" \
        b"\x62\x47\x63\x69\x4f\x69\x4a\x6b\x61\x58\x49\x69\x4c\x43\x4a\x6c" \
        b"\x62\x6d\x4d\x69\x4f\x69\x4a\x42\x4d\x54\x49\x34\x51\x30\x4a\x44" \
        b"\x4c\x55\x68\x54\x4d\x6a\x55\x32\x49\x6e\x30\x2e\x2e\x74\x4f\x2d" \
        b"\x68\x78\x70\x56\x51\x6a\x56\x69\x50\x4e\x70\x75\x45\x4c\x46\x71" \
        b"\x72\x50\x77\x2e\x77\x4c\x77\x4c\x6c\x30\x50\x75\x4d\x53\x36\x6f" \
        b"\x4d\x6d\x41\x48\x50\x76\x6c\x5a\x71\x74\x64\x4e\x59\x30\x78\x6b" \
        b"\x62\x4f\x6e\x49\x69\x30\x4f\x43\x7a\x30\x69\x4f\x76\x4a\x6f\x4c" \
        b"\x62\x76\x69\x4f\x2d\x57\x42\x67\x59\x62\x56\x52\x57\x6b\x71\x75" \
        b"\x6d\x76\x53\x33\x63\x61\x6d\x76\x66\x45\x66\x53\x54\x6d\x54\x72" \
        b"\x6b\x79\x4d\x5a\x57\x4f\x69\x38\x78\x6a\x4a\x44\x30\x4b\x53\x76" \
        b"\x55\x4c\x43\x4f\x33\x58\x54\x74\x43\x4a\x53\x6d\x6c\x2d\x45\x2e" \
        b"\x74\x39\x2d\x52\x70\x45\x34\x6a\x31\x78\x4c\x44\x56\x2d\x61\x34" \
        b"\x6f\x48\x6a\x6d\x74\x67\x00\x00\x00\x06\x00\x13\x64\x6b\x76\x63" \
        b"\x32\x31\x65\x65\x65\x63\x76\x69\x76\x33\x3a\x63\x50\x74\x51\x00" \
        b"\xc0\x57\x00\x04\x00\x03\x00\x0a\x80\x29\x00\x08\x6d\xd2\x8d\x3d" \
        b"\x57\x68\x52\x0a\x00\x24\x00\x04\x6e\x7d\x1e\xff\x00\x08\x00\x14" \
        b"\x8c\x64\x03\xd2\x31\xf1\x1d\x99\xb0\x56\x4a\x2c\x5e\x3e\x42\x1e" \
        b"\x9b\x19\x4c\x8b\x80\x28\x00\x04\x29\xe4\x6f\x38"

    pkt = eth/ip/udp/GTP_U_Header(teid=teid)/inet_p/payload
    return bytes(pkt)


def gen_gtpu_sequpdate_args(max_session, start_ue_ip, ue_ip_offset, start_teid):
    kwargs = {"fields": [
        {'offset': 46, 'size': 4, 'min': start_teid,
         'max': start_teid+max_session-1},
        {'offset': ue_ip_offset, 'size': 4, 'min': ip2long(start_ue_ip),
         'max': ip2long(start_ue_ip)+max_session-1}]}
    return kwargs
