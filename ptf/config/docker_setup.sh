#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

set -ex
# TCP port of bess/web monitor
gui_port=8000
bessd_port=10514
metrics_port=8080

# Driver options. Choose any one of the three
#
# "dpdk" set as default
# "af_xdp" uses AF_XDP sockets via DPDK's vdev for pkt I/O. This version is non-zc version. ZC version still needs to be evaluated.
# "af_packet" uses AF_PACKET sockets via DPDK's vdev for pkt I/O.
# "sim" uses Source() modules to simulate traffic generation
mode="dpdk"
#mode="af_xdp"
#mode="af_packet"
# mode="sim"

# Gateway interface(s)
#
# In the order of ("s1u" "sgi")
# ifaces=("access" "core")
ifaces=("core" "access")
# ifaces=("enp3s0f0" "enp3s0f1")

# Static IP addresses of gateway interface(s) in cidr format
#
# In the order of (s1u sgi)
ipaddrs=(198.18.0.1/30 198.19.0.1/30)

# MAC addresses of gateway interface(s)
#
# In the order of (s1u sgi)
macaddrs=(0c:c4:7a:19:6d:ca 0c:c4:7a:19:6d:cb)

# Static IP addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u n-sgi)
nhipaddrs=(198.18.0.2 198.19.0.2)

# Static MAC addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u n-sgi)
nhmacaddrs=(22:53:7a:15:58:50 22:53:7a:15:58:50)

# IPv4 route table entries in cidr format per port
#
# In the order of ("{r-s1u}" "{r-sgi}")
routes=("11.1.1.128/25" "0.0.0.0/0")

num_ifaces=${#ifaces[@]}
num_ipaddrs=${#ipaddrs[@]}

# Set up static route and neighbor table entries of the SPGW
function setup_trafficgen_routes() {
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec pause ip neighbor add "${nhipaddrs[$i]}" lladdr "${nhmacaddrs[$i]}" dev "${ifaces[$i % num_ifaces]}"
		routelist=${routes[$i]}
		for route in $routelist; do
			sudo ip netns exec pause ip route add "$route" via "${nhipaddrs[$i]}" metric 100
		done
	done
}

# Assign IP address(es) of gateway interface(s) within the network namespace
function setup_addrs() {
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec pause ip addr add "${ipaddrs[$i]}" dev "${ifaces[$i % $num_ifaces]}"
	done
}

# Set up mirror links to communicate with the kernel
#
# These vdev interfaces are used for ARP + ICMP updates.
# ARP/ICMP requests are sent via the vdev interface to the kernel.
# ARP/ICMP responses are captured and relayed out of the dpdk ports.
function setup_mirror_links() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip netns exec pause ip link add "${ifaces[$i]}" type veth peer name "${ifaces[$i]}"-vdev
		sudo ip netns exec pause ip link set "${ifaces[$i]}" up
		sudo ip netns exec pause ip link set "${ifaces[$i]}-vdev" up
		sudo ip netns exec pause ip link set dev "${ifaces[$i]}" address "${macaddrs[$i]}"
	done
	setup_addrs
}

# Set up interfaces in the network namespace. For non-"dpdk" mode(s)
function move_ifaces() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip link set "${ifaces[$i]}" netns pause up
		sudo ip netns exec pause ip link set "${ifaces[$i]}" promisc off
		sudo ip netns exec pause ip link set "${ifaces[$i]}" xdp off
		if [ "$mode" == 'af_xdp' ]; then
			sudo ip netns exec pause ethtool --features "${ifaces[$i]}" ntuple off
			sudo ip netns exec pause ethtool --features "${ifaces[$i]}" ntuple on
			sudo ip netns exec pause ethtool -N "${ifaces[$i]}" flow-type udp4 action 0
			sudo ip netns exec pause ethtool -N "${ifaces[$i]}" flow-type tcp4 action 0
			sudo ip netns exec pause ethtool -u "${ifaces[$i]}"
		fi
	done
	setup_addrs
}

# Stop previous instances of bess* before restarting
docker stop pause bess bess-routectl bess-web bess-pfcpiface || true
docker rm -f pause bess bess-routectl bess-web bess-pfcpiface || true
sudo rm -rf /var/run/netns/pause

# Build
make docker-build

if [ "$mode" == 'dpdk' ]; then
	DEVICES=${DEVICES:-'--device=/dev/vfio/48 --device=/dev/vfio/49 --device=/dev/vfio/vfio'}
	PRIVS='--cap-add IPC_LOCK'

elif [ "$mode" == 'af_xdp' ]; then
	PRIVS='--privileged'

elif [ "$mode" == 'af_packet' ]; then
	PRIVS='--cap-add IPC_LOCK'
fi

# Run pause
docker run --name pause -td --restart unless-stopped \
	-p $bessd_port:$bessd_port \
	-p $gui_port:$gui_port \
	-p $metrics_port:$metrics_port \
	--hostname $(hostname) \
	k8s.gcr.io/pause

# Emulate CNI + init container
sudo mkdir -p /var/run/netns
sandbox=$(docker inspect --format='{{.NetworkSettings.SandboxKey}}' pause)
sudo ln -s "$sandbox" /var/run/netns/pause

case $mode in
"dpdk" | "sim") setup_mirror_links ;;
"af_xdp" | "af_packet")
	move_ifaces
	# Make sure that kernel does not send back icmp dest unreachable msg(s)
	sudo ip netns exec pause iptables -I OUTPUT -p icmp --icmp-type port-unreachable -j DROP
	;;
*) ;;

esac

# Setup trafficgen routes
if [ "$mode" != 'sim' ]; then
	setup_trafficgen_routes
fi

# Run bessd
# --cpuset-cpus=8-9 \
# -u 0 \
# --cap-add IPC_LOCK
# -m 2048
# --privileged \
# --cap-add=all \
docker run --name bess -td --restart unless-stopped \
	--cpuset-cpus=5-8 \
	--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
	-v "$PWD/conf":/opt/bess/bessctl/conf \
	--net container:pause \
	$PRIVS \
	$DEVICES \
	--privileged \
	upf-epc-bess:"$(<VERSION)" -grpc-url=0.0.0.0:$bessd_port -v=1

docker logs bess

# Sleep for a couple of secs before setting up the pipeline
sleep 8
docker exec bess ./bessctl run up4
sleep 5

# Run bess-web
docker run --name bess-web -d --restart unless-stopped \
	--net container:bess \
	--entrypoint bessctl \
	upf-epc-bess:"$(<VERSION)" http 0.0.0.0 $gui_port

# Run bess-pfcpiface depending on mode type
# FIXME: remove dependency on pfcpiface when running ptf tests
# modify ptf to do initial setup instead of pfcpiface
docker run --name bess-pfcpiface -td --restart on-failure \
	--net container:pause \
	-v "$PWD/conf/upf.json":/conf/upf.json \
	upf-epc-pfcpiface:"$(<VERSION)" \
	-config /conf/upf.json

# Don't run any other container if mode is "sim"
if [ "$mode" == 'sim' ]; then
	exit
fi

# Run bess-routectl
docker run --name bess-routectl -td --restart unless-stopped \
	-v "$PWD/conf/route_control.py":/route_control.py \
	--net container:pause --pid container:bess \
	--entrypoint /route_control.py \
	upf-epc-bess:"$(<VERSION)" -i "${ifaces[@]}"
