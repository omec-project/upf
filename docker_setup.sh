#!/usr/bin/env bash

ifaces=("s1u" "sgi")
gui_port=8000

docker stop bess bess-routectl bess-web
docker rm -f bess bess-routectl bess-web

docker build -t krsna1729/spgwu .

docker run --name bess -td --restart unless-stopped \
	--cap-add NET_ADMIN \
	--cpuset-cpus=12-13 \
	--device=/dev/vfio/48 --device=/dev/vfio/82 --device=/dev/vfio/vfio \
	--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
	-v "$PWD/conf":/conf \
	-p $gui_port:$gui_port \
	krsna1729/spgwu

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
