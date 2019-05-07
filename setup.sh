#!/bin/bash

docker build -t krsna1729/spgwu .

docker stop bess

docker run --name bess -itd --rm --cap-add NET_ADMIN \
--cpuset-cpus=12-13 \
--device=/dev/vfio/48 --device=/dev/vfio/82 --device=/dev/vfio/vfio \
--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
-v $(pwd):/conf \
krsna1729/spgwu

docker exec bess bash -c "
ip link add s1u type veth peer name s1u-vdev;
ip link add sgi type veth peer name sgi-vdev;
ip link set s1u up;
ip link set s1u-vdev up;
ip link set sgi up;
ip link set sgi-vdev up;
ip addr add 198.18.0.1/30 dev s1u;
ip addr add 198.19.0.1/30 dev sgi;
ip route;
"
