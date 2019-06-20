#!/usr/bin/env bash

# Update as per test environment
mode="dpdk" #"afpkt"
orig_ifaces=("eth1" "eth2")
ifaces=("s1u" "sgi")
macvlan=("enp24s0f0" "enp24s0f1")
ipaddrs=(198.18.0.1/30 198.19.0.1/30)
macaddrs=(3c:fd:fe:b4:41:90 3c:fd:fe:b4:41:91)
nhipaddrs=(198.18.0.2 198.19.0.2)
nhmacaddrs=(68:05:ca:31:fa:7a 68:05:ca:31:fa:7b)
routes=("11.1.1.128/27 11.1.1.160/27 11.1.1.192/27 11.1.1.224/27" "13.1.1.128/27 13.1.1.160/27 13.1.1.192/27 13.1.1.224/27")
len=${#ifaces[@]}

function setup_trafficgen_routes() {
	for ((i = 0; i < len; i++)); do
		ip neighbor add ${nhipaddrs[$i]} lladdr ${nhmacaddrs[$i]} dev ${ifaces[$i]}
		routelist=${routes[$i]}
		for route in $(echo $routelist); do
			ip route add $route via ${nhipaddrs[$i]}
		done
	done
}

function setup_mirror_links() {
	for ((i = 0; i < len; i++)); do
		ip link add ${ifaces[$i]} type veth peer name ${ifaces[$i]}-vdev
		ip link set ${ifaces[$i]} up
		ip link set ${ifaces[$i]}-vdev up
		ip addr add ${ipaddrs[$i]} dev ${ifaces[$i]}
		ip link set dev ${ifaces[$i]} address ${macaddrs[$i]}
	done
}

function rename_ifaces() {
	for ((i = 0; i < len; i++)); do
		ip link set ${orig_ifaces[$i]} down
		ip link set ${orig_ifaces[$i]} name ${ifaces[$i]} up
	done
}

(return 2>/dev/null) && echo "Sourced" && return

case $mode in
    # Rename ifaces
    ("afpkt") rename_ifaces
	      # Make sure that kernel does not send back icmp dest unreachable msg(s)
	      iptables -I OUTPUT -p icmp --icmp-type destination-unreachable -j DROP
	      ;;
    # Setup slow path to kernel
    ("dpdk") setup_mirror_links ;;
    (*) echo "mode var not set. Set it to either \"dpdk\" or \"afpkt\"."
	exit
	;;
esac

# Setup routes and neighbors for il_trafficgen test
#setup_trafficgen_routes
