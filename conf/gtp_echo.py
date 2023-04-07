# SPDX-License-Identifier: Apache-2.0
# Copyright 2022 Intel Corporation

import scapy.all as scapy
from scapy.contrib.gtp import *
from scapy.packet import *

def gtp_echo_request(src_ip):
  #use scapy to build a GTP-U Echo Request packet template
  eth = scapy.Ether()
  ip = scapy.IP(src=src_ip) # dst IP is overwritten
  udp = scapy.UDP(sport=2152, dport=2152)
  gtp = GTPHeader(gtp_type=1, seq=0)
  gtp_echo = GTPEchoRequest()
  pkt = eth/ip/udp/gtp/gtp_echo
  min_pkt_size = 60
  if len(pkt) < min_pkt_size:
    pad_len = min_pkt_size - len(pkt)
    pad = Padding()
    pad.load = '\x00' * pad_len
    pkt = pkt/pad

  return bytes(pkt)
