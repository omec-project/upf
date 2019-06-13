#!/usr/bin/env bash

# Update as per test environment
ifaces=("s1u" "sgi")
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

setup_mirror_links
setup_trafficgen_routes
