#!/usr/bin/env bash

# Start route_control daemon
nohup /conf/route_control.py &

# First add static arp table entries (change IP/MAC addresses accordingly)
arp -s 13.1.1.2 68:05:ca:31:fa:7b
arp -s 11.1.1.2 68:05:ca:31:fa:7a

# Next add route table entries (change IP addresses accordingly)
ip route add 13.1.1.128/25 via 13.1.1.2
ip route add 11.1.1.128/25 via 11.1.1.2
