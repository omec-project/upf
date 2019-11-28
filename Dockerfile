# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2019 Intel Corporation

# Multi-stage Dockerfile
# Stage bess-build: builds bess with its dependencies
FROM nefelinetworks/bess_build AS bess-build
RUN echo "deb-src http://archive.ubuntu.com/ubuntu/ bionic main restricted universe multiverse" >>/etc/apt/sources.list && \
    apt-get update && \
    apt-get -y build-dep libelf-dev && \
    apt-get -y install --no-install-recommends \
        build-essential \
        ca-certificates \
        git \
        libelf-dev \
        libnuma-dev \
        pkg-config \
        unzip \
        wget

ARG MAKEFLAGS

ENV LIBBPF_DIR="/libbpf"
RUN git clone https://github.com/libbpf/libbpf.git $LIBBPF_DIR
RUN cd $LIBBPF_DIR && \
    make $MAKEFLAGS -C src install && \
    cp include/uapi/linux/if_xdp.h /usr/include/linux

# dpdk
ARG DPDK_URL='http://dpdk.org/git/dpdk'
ARG DPDK_VER='v19.08'
ENV DPDK_DIR="/dpdk"
RUN git clone -b $DPDK_VER -q --depth 1 $DPDK_URL $DPDK_DIR 2>&1
ARG RTE_TARGET='x86_64-native-linuxapp-gcc'
RUN cd $DPDK_DIR && \
    sed -ri 's,(IGB_UIO=).*,\1n,' config/common_linux* && \
    sed -ri 's,(KNI_KMOD=).*,\1n,' config/common_linux* && \
    sed -ri 's,(LIBRTE_BPF=).*,\1n,' config/common_base && \
    sed -ri 's,(LIBRTE_PMD_PCAP=).*,\1y,' config/common_base && \
    sed -ri 's,(PORT_PCAP=).*,\1y,' config/common_base && \
    sed -ri 's,(AF_XDP=).*,\1y,' config/common_base && \
    make config T=$RTE_TARGET && \
    make $MAKEFLAGS EXTRA_CFLAGS="-g -w -fPIC"

# Workaround for compiler error on including DPDK in bess
WORKDIR $DPDK_DIR
COPY patches/dpdk patches
RUN cat patches/* | patch -p1 && \
    make $MAKEFLAGS EXTRA_CFLAGS="-g -w -fPIC"

WORKDIR /
ARG BESS_COMMIT=master
RUN wget -qO bess.zip https://github.com/NetSys/bess/archive/$BESS_COMMIT.zip && unzip bess.zip
WORKDIR bess-$BESS_COMMIT
COPY core/modules/ core/modules/
COPY core/utils/ core/utils/
COPY protobuf/ protobuf/
COPY patches/bess patches
RUN cp -a $DPDK_DIR deps/dpdk-17.11 && \
    cat patches/* | patch -p1
RUN CXXARCHFLAGS="-march=native -Werror=format-truncation -Warray-bounds -fbounds-check -fno-strict-overflow -fno-delete-null-pointer-checks -fwrapv" ./build.py bess && \
    cp bin/bessd /bin && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -a protobuf /protobuf

# Stage pip: compile psutil
FROM python:2.7-slim as pip
RUN apt-get update && apt-get install -y gcc
RUN pip install --no-cache-dir psutil

# Stage bess: creates the runtime image of BESS
FROM python:2.7-slim as bess
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgraph-easy-perl \
        iproute2 \
        iptables \
	iputils-ping \
        tcpdump && \
    rm -rf /var/lib/apt/lists/* && \
    pip install --no-cache-dir \
        flask \
        grpcio \
        iptools \
        protobuf \
        pyroute2 \
        https://github.com/secdev/scapy/archive/b65e795c62accd383e1bb6b17cd9f7a9143ae117.zip
COPY --from=pip /usr/local/lib/python2.7/site-packages/psutil /usr/local/lib/python2.7/site-packages/psutil
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin
RUN ln -s /opt/bess/bessctl/bessctl /bin
ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

# Compile cpiface
FROM ubuntu:18.04 as cpiface-build
RUN apt-get update && apt-get install -y \
    autoconf \
    automake \
    clang \
    build-essential \
    git \
    libc++-dev \
    libgflags-dev \
    libgoogle-glog-dev \
    libgtest-dev \
    libtool \
    libzmq3-dev \
    pkg-config
ARG MAKEFLAGS
RUN cd /opt && \
    git clone -q https://github.com/grpc/grpc.git && \
    cd grpc && \
    git checkout 216fa1cab3a42edb2e6274b67338351aade99a51 && \
    git submodule update --init && \
    make $MAKEFLAGS && \
    echo "/opt/grpc/libs/opt" > /etc/ld.so.conf.d/grpc.conf && \
    ldconfig
ENV PATH=$PATH:/opt/grpc/bins/opt/:/opt/grpc/bins/opt/protobuf
COPY cpiface /cpiface
COPY --from=bess-build /protobuf /protobuf
# Copying explicitly since symlinks don't work
COPY core/utils/gtp_common.h /cpiface
RUN cd /cpiface && \
    make $MAKEFLAGS PBDIR=/protobuf && \
    cp zmq-cpiface /bin/

# Stage cpiface: creates runtime image of cpiface
FROM ubuntu:18.04 as cpiface
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libzmq5 \
	libgoogle-glog0v5 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=cpiface-build /opt/grpc/libs/opt /opt/grpc/libs/opt
RUN echo "/opt/grpc/libs/opt" > /etc/ld.so.conf.d/grpc.conf && \
    ldconfig
COPY --from=cpiface-build /bin/zmq-cpiface /bin
