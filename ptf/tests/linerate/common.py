# SPDX-License-Identifier: Apache-2.0
# Copyright 2024 Intel Corporation

from ipaddress import IPv4Address

# MAC addresses
TREX_SRC_MAC = "40:a6:b7:20:c8:25" # Source MAC address for DL traffic
UPF_CORE_MAC = "40:a6:b7:20:4f:b9" # MAC address of N6 for the UPF/BESS
UPF_ACCESS_MAC = "40:a6:b7:20:4f:b8" # MAC address of N3 for the UPF/BESS

# Port setup
TREX_SENDER_PORT = 1
TREX_RECEIVER_PORT = 0
UPF_CORE_PORT = 1
UPF_ACCESS_PORT = 0

# test specs
DURATION = 10
RATE = 100_000  # 100 Kpps
UE_COUNT = 10_000  # 10k UEs
PKT_SIZE = 64
PKT_SIZE_L = 1400 # Packet size for MBR test

# IP addresses
UE_IP_START = IPv4Address("16.0.0.1")
GNB_IP = IPv4Address("11.1.1.129")
N3_IP = IPv4Address("198.18.0.1")
PDN_IP = IPv4Address("6.6.6.6") # Must be routable by route_control

