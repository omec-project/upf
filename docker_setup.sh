#!/usr/bin/env bash
set -e
gui_port=8000
# Update as per test environment
mode="dpdk" # "af_xdp" "af_packet"
ifaces=("ens803f2" "ens803f3")
ipaddrs=(198.18.0.1/30 198.19.0.1/30)
macaddrs=(68:05:ca:33:2e:20 68:05:ca:33:2e:21)
nhipaddrs=(198.18.0.2 198.19.0.2)
nhmacaddrs=(68:05:ca:31:fa:7a 68:05:ca:31:fa:7b)
routes=("11.1.1.128/27 11.1.1.160/27 11.1.1.192/27 11.1.1.224/27" "13.1.1.128/27 13.1.1.160/27 13.1.1.192/27 13.1.1.224/27")
num_ifaces=${#ifaces[@]}
num_ipaddrs=${#ipaddrs[@]}

function setup_trafficgen_routes() {
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec bess ip neighbor add "${nhipaddrs[$i]}" lladdr "${nhmacaddrs[$i]}" dev "${ifaces[$i%num_ifaces]}"
		routelist=${routes[$i]}
		for route in $routelist; do
			sudo ip netns exec bess ip route add "$route" via "${nhipaddrs[$i]}"
		done
	done
}

function setup_addrs(){
	for ((i = 0; i < num_ipaddrs; i++)); do
		sudo ip netns exec bess ip addr add "${ipaddrs[$i]}" dev "${ifaces[$i%$num_ifaces]}"
	done
}

function setup_mirror_links() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip netns exec bess ip link add "${ifaces[$i]}" type veth peer name "${ifaces[$i]}"-vdev
		sudo ip netns exec bess ip link set "${ifaces[$i]}" up
		sudo ip netns exec bess ip link set "${ifaces[$i]}-vdev" up
		sudo ip netns exec bess ip link set dev "${ifaces[$i]}" address "${macaddrs[$i]}"
	done
	setup_addrs
}

function move_ifaces() {
	for ((i = 0; i < num_ifaces; i++)); do
		sudo ip link set "${ifaces[$i]}" netns bess up
	done
	setup_addrs
}

docker stop bess bess-routectl bess-web || true
docker rm -f bess bess-routectl bess-web || true
sudo rm -rf /var/run/netns/bess

docker build --pull -t krsna1729/spgwu .

[ "$mode" == 'dpdk' ] && DEVICES=${DEVICES:-'--device=/dev/vfio/48 --device=/dev/vfio/49 --device=/dev/vfio/vfio'} || DEVICES=''
[ "$mode" == 'af_xdp' ] && PRIVS='--privileged' || PRIVS='--cap-add NET_ADMIN'

docker run --name bess -td --restart unless-stopped \
	--cpuset-cpus=12-13 \
	--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
	-v "$PWD/conf":/opt/bess/bessctl/conf \
	-p $gui_port:$gui_port \
	$PRIVS \
	$DEVICES \
	krsna1729/spgwu

sudo mkdir -p /var/run/netns
sandbox=$(docker inspect --format='{{.NetworkSettings.SandboxKey}}' bess)
sudo ln -s "$sandbox" /var/run/netns/bess

case $mode in
"dpdk") setup_mirror_links ;;
*)
	move_ifaces
	# Make sure that kernel does not send back icmp dest unreachable msg(s)
	sudo ip netns exec bess iptables -I OUTPUT -p icmp --icmp-type port-unreachable -j DROP
	;;
esac

# Setup trafficgen routes
setup_trafficgen_routes

docker logs bess

docker run --name bess-routectl -td --restart unless-stopped \
	-v "$PWD/conf/route_control.py":/route_control.py \
	--net container:bess --pid container:bess \
	--entrypoint /route_control.py \
	krsna1729/spgwu -i "${ifaces[@]}"

docker run --name bess-web -d --restart unless-stopped \
	--net container:bess \
	--entrypoint bessctl \
	krsna1729/spgwu http 0.0.0.0 $gui_port
