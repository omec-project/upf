#!/usr/bin/env bash

# update ifaces, ipaddrs, macaddrs, & ipmask accordingly
ifaces=( "s1u" "sgi" )
ipaddrs=( 198.18.0.1 198.19.0.1 )
macaddrs=( 3c:fd:fe:b4:41:90 3c:fd:fe:b4:41:91 )
ipmask=30
len=${#ifaces[@]}

docker build -t krsna1729/spgwu .

docker stop bess
docker rm -f bess

docker run --name bess -itd --cap-add NET_ADMIN \
--cpuset-cpus=12-13 \
--device=/dev/vfio/48 --device=/dev/vfio/82 --device=/dev/vfio/vfio \
--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages \
-v "$PWD/conf":/conf \
krsna1729/spgwu

for (( i=0; i<$len; i++ ))
do
docker exec bess bash -c "
ip link add ${ifaces[$i]} type veth peer name ${ifaces[$i]}-vdev;
ip link set ${ifaces[$i]} up;
ip link set ${ifaces[$i]}-vdev up;
ip addr add ${ipaddrs[$i]}/$ipmask dev ${ifaces[$i]};
ip link set dev ${ifaces[$i]} address ${macaddrs[$i]}
"
done

docker exec bess bash -c "
ip route
"

docker logs bess

docker stop bess-routectl
docker rm -f bess-routectl

docker run --name bess-routectl -itd \
-v "$PWD/conf":/conf \
--net container:bess \
--entrypoint /conf/route_control.py \
krsna1729/spgwu -i "${ifaces[@]}"
