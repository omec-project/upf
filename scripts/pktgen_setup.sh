#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2023-Present Intel Corporation

set -e

# Namespaced pktgen resources.
#
# The traffic generator reuses the same BESS image and pipeline as the UPF,
# so its containers, network namespace, and host ports are prefixed/offset to
# avoid colliding with a UPF deployment (scripts/docker_setup.sh) when both run
# on the same host (e.g. the extra-VF-on-the-same-machine setup). Without this,
# the "docker stop/rm" below and the published ports would tear down the UPF
# under test.
pause_name=pktgen-pause
bess_name=pktgen-bess
web_name=pktgen-web
routectl_name=pktgen-routectl
netns=pktgen

# Container-internal ports. Kept at the BESS defaults so bessctl and
# route_control connect without extra flags.
gui_port=8000
bessd_port=10514
metrics_port=8080

# Host-published ports. Offset from the BESS defaults so the published ports
# do not clash with a UPF deployment on the same host.
host_gui_port=8001
host_bessd_port=10515
host_metrics_port=8081

# Path to the pktgen config consumed by conf/pktgen.bess.
conf_file="${CONF_FILE:-conf/pktgen.jsonc}"

# Driver mode. Read from pktgen.jsonc so the namespace topology built here
# always matches the port driver selected by the pipeline (conf/pktgen.bess):
#
# "dpdk" (default) sets up DPDK mirror veths.
# "af_xdp" uses AF_XDP sockets via DPDK's vdev for pkt I/O. This version is non-zc version. ZC version still needs to be evaluated.
# "af_packet" uses AF_PACKET sockets via DPDK's vdev for pkt I/O.
#
# Editing the single "mode" value in pktgen.jsonc keeps setup and pipeline in
# sync; a mismatch would create the wrong topology (e.g. mirror veths while the
# pipeline expects a moved NIC) and the generator could not reach the datapath.
mode=$(grep -v '^[[:space:]]*//' "$conf_file" \
	| sed -n 's/.*"mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
	| head -1)
mode="${mode:-dpdk}"
echo "pktgen driver mode (from $conf_file): $mode"

# Gateway interface(s)
#
# In the order of ("s1u/n3" "sgi/n6")
ifaces=("ens785f0" "ens785f1")

# Static IP addresses of gateway interface(s) in cidr format
#
# In the order of (s1u/n3 sgi/n6)
ipaddrs=(198.18.0.2/30 198.19.0.2/30)

# MAC addresses of gateway interface(s)
#
# In the order of (s1u/n3 sgi/n6)
macaddrs=(b4:96:91:b4:44:b0 b4:96:91:b4:44:b1)

# Static IP addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u/n3 n-sgi/n6)
nhipaddrs=(198.18.0.1 198.19.0.1)

# Static MAC addresses of the neighbors of gateway interface(s)
#
# In the order of (n-s1u/n3 n-sgi/n6)
nhmacaddrs=(b4:96:91:b4:47:b8 b4:96:91:b4:47:b9)

# IPv4 route table entries in cidr format per port
#
# In the order of ("{r-s1u/n3}" "{r-sgi/n6}")
routes=("21.1.1.128/25" "0.0.0.0/0")

num_ifaces=${#ifaces[@]}
num_ipaddrs=${#ipaddrs[@]}

# Set up static route and neighbor table entries of the SPGW/UPF
function setup_trafficgen_routes() {
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec "$netns" ip neighbor add "${nhipaddrs[$i]}" lladdr "${nhmacaddrs[$i]}" dev "${ifaces[$i % num_ifaces]}"
		routelist=${routes[$i]}
		for route in $routelist; do
			sudo ip netns exec "$netns" ip route add "$route" via "${nhipaddrs[$i]}" metric 100
		done
	done
}

# Assign IP address(es) of gateway interface(s) within the network namespace
function setup_addrs() {
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec "$netns" ip addr add "${ipaddrs[$i]}" dev "${ifaces[$i % $num_ifaces]}"
	done
}

# Set up mirror links to communicate with the kernel
#
# These vdev interfaces are used for ARP + ICMP updates.
# ARP/ICMP requests are sent via the vdev interface to the kernel.
# ARP/ICMP responses are captured and relayed out of the dpdk ports.
function setup_mirror_links() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip netns exec "$netns" ip link add "${ifaces[$i]}" type veth peer name "${ifaces[$i]}"-vdev
		sudo ip netns exec "$netns" ip link set "${ifaces[$i]}" up
		sudo ip netns exec "$netns" ip link set "${ifaces[$i]}-vdev" up
		sudo ip netns exec "$netns" ip link set dev "${ifaces[$i]}" address "${macaddrs[$i]}"
	done
	setup_addrs
}

# Set up interfaces in the network namespace. For non-"dpdk" mode(s)
function move_ifaces() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip link set "${ifaces[$i]}" netns "$netns" up
		sudo ip netns exec "$netns" ip link set "${ifaces[$i]}" promisc off
		sudo ip netns exec "$netns" ip link set "${ifaces[$i]}" xdp off
		if [ "$mode" == 'af_xdp' ]; then
			sudo ip netns exec "$netns" ethtool --features "${ifaces[$i]}" ntuple off
			sudo ip netns exec "$netns" ethtool --features "${ifaces[$i]}" ntuple on
			sudo ip netns exec "$netns" ethtool -N "${ifaces[$i]}" flow-type udp4 action 0
			sudo ip netns exec "$netns" ethtool -N "${ifaces[$i]}" flow-type tcp4 action 0
			sudo ip netns exec "$netns" ethtool -u "${ifaces[$i]}"
		fi
	done
	setup_addrs
}

# Stop previous instances of the pktgen containers before restarting
docker stop "$pause_name" "$bess_name" "$routectl_name" "$web_name" || true
docker rm -f "$pause_name" "$bess_name" "$routectl_name" "$web_name" || true
sudo rm -rf /var/run/netns/"$netns"

# Build
make docker-build

if [ "$mode" == 'dpdk' ]; then
	DEVICES=${DEVICES:-'--device=/dev/vfio/184 --device=/dev/vfio/185 --device=/dev/vfio/vfio'}
	PRIVS='--cap-add IPC_LOCK'

elif [ "$mode" == 'af_xdp' ]; then
	PRIVS='--privileged'

elif [ "$mode" == 'af_packet' ]; then
	PRIVS='--cap-add IPC_LOCK'
fi

# Run pause
docker run --name "$pause_name" -td --restart unless-stopped \
	-p $host_bessd_port:$bessd_port \
	-p $host_gui_port:$gui_port \
	-p $host_metrics_port:$metrics_port \
	--hostname $(hostname) \
	k8s.gcr.io/pause

# Emulate CNI + init container
sudo mkdir -p /var/run/netns
sandbox=$(docker inspect --format='{{.NetworkSettings.SandboxKey}}' "$pause_name")
sudo ln -s "$sandbox" /var/run/netns/"$netns"

case $mode in
"dpdk" | "sim") setup_mirror_links ;;
"af_xdp" | "af_packet")
	move_ifaces
	# Make sure that kernel does not send back icmp dest unreachable msg(s)
	sudo ip netns exec "$netns" iptables -I OUTPUT -p icmp --icmp-type port-unreachable -j DROP
	;;
*) ;;

esac

# Setup trafficgen routes
if [ "$mode" != 'sim' ]; then
	setup_trafficgen_routes
fi

# Run bessd
docker run --name "$bess_name" -td --restart unless-stopped \
	-v /lib/firmware/intel/ice/ddp:/lib/firmware/intel/ice/ddp \
	--cpuset-cpus=54-71 \
	--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
	-v "$PWD/conf":/opt/bess/bessctl/conf \
	--net container:"$pause_name" \
	$PRIVS \
	$DEVICES \
	upf-bess:"$(<VERSION)" -grpc-url=0.0.0.0:$bessd_port

docker logs "$bess_name"

# Sleep for a couple of secs before setting up the pipeline
sleep 10
docker exec -e CONF_FILE="$conf_file" "$bess_name" ./bessctl run pktgen
sleep 10

# Run bess-web
docker run --name "$web_name" -d --restart unless-stopped \
	--net container:"$bess_name" \
	--entrypoint bessctl \
	upf-bess:"$(<VERSION)" http 0.0.0.0 $gui_port

# Don't run any other container if mode is "sim"
if [ "$mode" == 'sim' ]; then
	exit
fi

# Run bess-routectl
docker run --name "$routectl_name" -td --restart unless-stopped \
	-v "$PWD/conf/route_control.py":/route_control.py \
	--net container:"$pause_name" --pid container:"$bess_name" \
	--entrypoint /route_control.py \
	upf-bess:"$(<VERSION)" -i "${ifaces[@]}"
