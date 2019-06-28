#!/usr/bin/env bash
set -e
gui_port=8000

source conf/setup.sh

function setup_docker_net() {
	for ((i = 0; i < len; i++)); do
		docker network rm ${ifaces[$i]}-net
		docker network create -d macvlan \
			--subnet=${ipaddrs[$i]} \
			--gateway=${nhipaddrs[$i]} \
			-o parent=${macvlan[$i]} ${ifaces[$i]}-net
		docker network connect ${ifaces[$i]}-net bess
	done
}

function run_bess_af_packet() {
	docker create --name bess -t --restart unless-stopped \
		--cap-add NET_ADMIN \
		--cpuset-cpus=12-13 \
		--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
		-v "$PWD/conf":/conf \
		-p $gui_port:$gui_port \
		krsna1729/spgwu
	setup_docker_net
	docker start bess
}

function run_bess_dpdk() {
	docker run --name bess -td --restart unless-stopped \
		--cap-add NET_ADMIN \
		--cpuset-cpus=12-13 \
		--device=/dev/vfio/48 --device=/dev/vfio/49 --device=/dev/vfio/vfio \
		--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
		-v "$PWD/conf":/conf \
		-p $gui_port:$gui_port \
		krsna1729/spgwu
}

docker stop bess bess-routectl bess-web || true
docker rm -f bess bess-routectl bess-web || true

docker build --pull -t krsna1729/spgwu .

case $mode in
    ("dpdk") echo "Running bessd with dpdk"
	     run_bess_dpdk ;;
    ("af_packet") echo "Running bessd with af_packet"
	      run_bess_af_packet ;;
    (*) echo "Control can never come here"
	exit ;;
esac

docker exec bess /conf/setup.sh
docker logs bess

docker run --name bess-routectl -td --restart unless-stopped \
	-v "$PWD/conf":/conf \
	--net container:bess --pid container:bess \
	--entrypoint /conf/route_control.py \
	krsna1729/spgwu -i "${ifaces[@]}"

docker run --name bess-web -d --restart unless-stopped \
	--net container:bess \
	--entrypoint bessctl \
	krsna1729/spgwu http 0.0.0.0 $gui_port
