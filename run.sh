docker stop bess

docker run --name bess -itd --rm --privileged \
--device=/dev/vfio/48 --device=/dev/vfio/49 --device=/dev/vfio/vfio \
--ulimit memlock=-1 -v /dev/hugepages:/dev/hugepages -v $(pwd):/router \
ngick8stesting/ngic-bessd-ctl:pkgs bessd -f

docker exec bess bash -c "
ip link add l_s1u type veth peer name s1u;
ip link add l_sgi type veth peer name sgi;
ip link set s1u up;
ip link set sgi up;
ip netns add blue;
ip link set l_s1u netns blue up;
ip link set l_sgi netns blue up;
ip netns exec blue ip addr add 1.1.1.10/24 dev l_s1u;
ip netns exec blue ip addr add 2.2.2.10/24 dev l_sgi;
ip netns exec blue ip route;
ip netns exec blue ip neigh;
"

