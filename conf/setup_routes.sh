#!/usr/bin/env bash

S1UDEV='s1u'
SGIDEV='sgi'
DEST_S1U_IP=11.1.1.2
DEST_SGI_IP=13.1.1.2
DEST_S1U_MAC=68:05:ca:31:fa:7a
DEST_SGI_MAC=68:05:ca:31:fa:7b
DEST_S1U_IP_RANGE=11.1.1.128/25
DEST_SGI_IP_RANGE=13.1.1.128/25

# Start route_control daemon
nohup /conf/route_control.py -i $S1UDEV $SGIDEV &

# First add static arp table entries (change IP/MAC addresses accordingly)
#arp -s $DEST_SGI_IP $DEST_SGI_MAC
#arp -s $DEST_S1U_IP $DEST_S1U_MAC

# Next add route table entries (change IP addresses accordingly)
#ip route add $DEST_SGI_IP_RANGE via $DEST_SGI_IP
#ip route add $DEST_S1U_IP_RANGE via $DEST_S1U_IP
