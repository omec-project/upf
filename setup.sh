#!/bin/bash

S1U_NAME="s1u"
SGI_NAME="sgi"
S1U_IP=198.18.0.1
SGI_IP=198.19.0.1
S1U_MAC=3c:fd:fe:b4:41:90
SGI_MAC=3c:fd:fe:b4:41:91
IPMASK=30

docker build -t krsna1729/spgwu .

docker stop bess
docker rm -f bess

docker run --name bess -itd --cap-add NET_ADMIN \
--cpuset-cpus=12-13 \
--device=/dev/vfio/48 --device=/dev/vfio/82 --device=/dev/vfio/vfio \
--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
-v "$PWD/conf":/conf \
krsna1729/spgwu

docker exec bess bash -c "
ip link add $S1U_NAME type veth peer name $S1U_NAME-vdev;
ip link add $SGI_NAME type veth peer name $SGI_NAME-vdev;
ip link set $S1U_NAME up;
ip link set $S1U_NAME-vdev up;
ip link set $SGI_NAME up;
ip link set $SGI_NAME-vdev up;
ip addr add $S1U_IP/$IPMASK dev $S1U_NAME;
ip addr add $SGI_IP/$IPMASK dev $SGI_NAME;
ip link set dev $S1U_NAME address $S1U_MAC
ip link set dev $SGI_NAME address $SGI_MAC
ip route;
"

docker logs bess
