#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2022 Open Networking Foundation

set -ex
# TCP port of bess/web monitor
gui_port=8000
bessd_port=10514
metrics_port=8080

# Driver options. Choose any one of the two:
# "dpdk" set as default
# "sim" uses Source() modules to simulate traffic generation
mode="dpdk"
# mode="sim"

# The veth interface used for packet io between BESS and PFCP agent.
veth_iface_name="fab"
veth_iface_ip="198.18.0.1/24"
veth_iface_gateway_ip="198.18.0.100"
veth_iface_gateway_mac="00:00:00:bb:bb:bb"
alt_route_table=201

# Set up mirror link to communicate with the kernel
# This vdev interface is used for ARP + ICMP + DHCP updates.
# ARP/ICMP/DHCP requests are sent via the vdev interface to the kernel.
# ARP/ICMP/DHCP responses are captured and relayed out of the dpdk ports.
function setup_veth_interface() {
	# Device setup.
	sudo ip netns exec pause ip link add "${veth_iface_name}" type veth peer name "${veth_iface_name}-vdev"
	sudo ip netns exec pause ip link set "${veth_iface_name}" up
	sudo ip netns exec pause ip link set "${veth_iface_name}-vdev" up

	# Address setup. Here we assign a static IP, but in other deployments this is DHCP assigned.
	sudo ip netns exec pause ip addr add "${veth_iface_ip}" dev "${veth_iface_name}"

	# Route setup.
	# Default route setup via fabric gateway with policy-based routing.
	sudo ip netns exec pause ip rule add from "${veth_iface_ip}" table "${alt_route_table}" prio 1
	sudo ip netns exec pause ip route add default via "${veth_iface_gateway_ip}" dev "${veth_iface_name}" table "${alt_route_table}"

	# Simulated GW neighbor setup.
	# sudo ip netns exec pause ip neigh add "${veth_iface_gateway_ip}" lladdr "${veth_iface_gateway_mac}" dev "${veth_iface_name}" nud permanent
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

setup_veth_interface

# Run bessd
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
	upf-epc-pfcpiface:"$(<VERSION)" \
	-config /conf/aether.json
#  -simulate create_continue

# Run simulator to create forwarding state.
docker container run --rm -d --net=container:pause --name pfcpsim pfcpsim:0.1.0-dev
docker exec pfcpsim pfcpctl -c configure --n3-addr 198.18.0.1 --remote-peer 127.0.0.1:8805
docker exec pfcpsim pfcpctl -c associate
docker exec pfcpsim pfcpctl -c create --count 1 --baseID 2 --ue-pool 10.250.0.0/29 --nb-addr 11.0.0.1

# Send some packets with scapy
# TODO(max): For development only, remove before merging

# Downlink packet
# sendp( (
#       Ether(src="00:00:00:11:11:11", dst="0c:c4:7a:19:6d:ca") /
#           IP(src="8.8.8.8", dst="10.250.0.1") /
#               UDP(sport=80, dport=8888) /
#                   ("A"*30)
#       ), iface='enp175s0f0', count=1)

# Uplink packet
# from scapy.contrib.gtp import *
# sendp( (
#          Ether(src="00:00:00:22:22:22", dst="0c:c4:7a:19:6d:ca") /
#          IP(src="11.0.0.1", dst="198.18.0.1") /
#          UDP(sport=2152, dport=2152) /
#          GTP_U_Header(gtp_type=255, teid=2) /
#          IP(src="10.250.0.1", dst="8.8.8.8") /
#          UDP(sport=8888, dport=80) /
#          ("B"*30)
#        ), iface='enp175s0f0', count=1)

# Inside netns pause:
# tcpdump -lenvvi fab

# ARPs from sim eNB:
# sudo arping -i enp175s0f0 -S 10.17.0.2 10.17.0.1
