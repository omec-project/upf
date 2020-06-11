#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

from scapy.contrib.gtp import *
from scapy.all import *


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

def gen_ue_packet(size, src_mac, dst_mac, src_ip, dst_ip, inner_src_ip, inner_dst_ip):
    eth = Ether(src=src_mac, dst=dst_mac)
    ip = IP(src=src_ip, dst=dst_ip)
    udp = UDP(sport=2152, dport=2152)
    inet_p = IP(src=inner_src_ip, dst=inner_dst_ip)/UDP()
    payload = ('hello' + '0123456789' * 200)[:size-len(eth/ip/udp/inet_p)]
    pkt = eth/ip/udp/GTP_U_Header(teid=0xf0000000)/inet_p/payload
    return bytes(pkt)
