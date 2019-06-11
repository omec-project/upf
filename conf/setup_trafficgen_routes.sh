#!/usr/bin/env bash

S1UDEV='s1u'
SGIDEV='sgi'
DEST_S1U_IP=198.18.0.2
DEST_SGI_IP=198.19.0.2
DEST_S1U_MAC=68:05:ca:31:fa:7a
DEST_SGI_MAC=68:05:ca:31:fa:7b
DEST_S1U_IP_RANGE=198.18.0.128/25
DEST_SGI_IP_RANGE=198.19.0.128/25

# First add static arp table entries (change IP/MAC addresses accordingly)
ip neighbor add $DEST_SGI_IP lladdr $DEST_SGI_MAC dev $SGIDEV
ip neighbor add $DEST_S1U_IP lladdr $DEST_S1U_MAC dev $S1UDEV

# Next add route table entries (change IP addresses accordingly)
ip route add $DEST_SGI_IP_RANGE via $DEST_SGI_IP
ip route add $DEST_S1U_IP_RANGE via $DEST_S1U_IP
