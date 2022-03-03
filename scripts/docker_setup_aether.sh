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
# mode="af_xdp"
# mode="af_packet"
# mode="sim"

# veth interface used for packet io between BESS and PFCP agent.
ifaces=("pktio")

# Static IP addresses of gateway interface(s) in cidr format
#
# In the order of (s1u sgi)
ipaddrs=(198.18.0.1/30)

# MAC addresses of gateway interface(s)
#
# In the order of (s1u sgi)
macaddrs=(0c:c4:7a:19:6d:ca)

# Static IP addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u n-sgi)
nhipaddrs=(198.18.0.2)

# Static MAC addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u n-sgi)
nhmacaddrs=(22:53:7a:15:58:50)

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
		sudo ip netns exec pause ip link add "${ifaces[$i]}" type veth peer name "${ifaces[$i]}"-dpdk
		sudo ip netns exec pause ip link set "${ifaces[$i]}" up
		sudo ip netns exec pause ip link set "${ifaces[$i]}-dpdk" up
		# sudo ip netns exec pause ip link set dev "${ifaces[$i]}" address "${macaddrs[$i]}"
	done
	setup_addrs
}

# Stop previous instances of bess* before restarting
docker stop pause bess bess-routectl bess-web bess-pfcpiface pfcpsim || true
docker rm -f pause bess bess-routectl bess-web bess-pfcpiface pfcpsim || true
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
	-p 8805:8805 \
	--hostname $(hostname) \
	k8s.gcr.io/pause

# Emulate CNI + init container
sudo mkdir -p /var/run/netns
sandbox=$(docker inspect --format='{{.NetworkSettings.SandboxKey}}' pause)
sudo ln -s "$sandbox" /var/run/netns/pause

case $mode in
"dpdk" | "sim") setup_mirror_links ;;
"af_xdp" | "af_packet")
	echo "Unsupported mode"
	exit 1
	;;
*) ;;

esac

# Setup trafficgen routes
if [ "$mode" != 'sim' ]; then
	setup_trafficgen_routes
fi

rm -rf /tmp/sockets
mkdir -p /tmp/sockets

# Run bessd
docker run --name bess -td --restart unless-stopped \
	--cpuset-cpus=5-8 \
	--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
	-v "$PWD/conf":/opt/bess/bessctl/conf \
	-v /tmp/sockets:/tmp/sockets \
	--net container:pause \
	$PRIVS \
	$DEVICES \
	--privileged \
	upf-epc-bess:"$(<VERSION)" -grpc-url=0.0.0.0:$bessd_port -v=1

docker logs bess

# Run bess-web
docker run --name bess-web -d --restart unless-stopped \
	--net container:bess \
	--entrypoint bessctl \
	upf-epc-bess:"$(<VERSION)" http 0.0.0.0 $gui_port

# Sleep for a couple of secs before setting up the pipeline
sleep 8
docker exec bess ./bessctl run aether
sleep 5

# Run bess-pfcpiface depending on mode type
docker run --name bess-pfcpiface -td --restart on-failure \
	--net container:pause \
	-v "$PWD/conf/aether.json":/conf/aether.json \
	-v /tmp/sockets:/tmp/sockets \
	upf-epc-pfcpiface:"$(<VERSION)" \
	-config /conf/aether.json
	#  -simulate create_continue


# Run simulator to create forwarding state.
docker container run --rm -d --net=container:pause --name pfcpsim pfcpsim:0.1.0-dev
docker exec pfcpsim pfcpctl -c configure --n3-addr 10.17.0.1 --remote-peer 127.0.0.1:8805
docker exec pfcpsim pfcpctl -c associate
docker exec pfcpsim pfcpctl -c create --count 1 --baseID 2 --ue-pool 10.250.0.0/29 --nb-addr 11.0.0.1

# Send some packets with scapy
# TODO(max): For development only, remove before merging

# Downlink packet
# sendp( (
#      Ether(src="00:00:00:11:11:11", dst="0c:c4:7a:19:6d:ca") /
#          IP(src="8.8.8.8", dst="10.250.0.1") /
#              UDP(sport=80, dport=8888) /
#                  ("A"*30)
#                  ), iface='enp175s0f0', count=1)

# Inside netns pause:
# tcpdump -lenvvi pktio

# ARPs from sim eNB:
# sudo arping -i enp175s0f0 -S 10.17.0.2 10.17.0.1
