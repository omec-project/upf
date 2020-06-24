# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright (c) 2019 Intel Corporation

# Multi-stage Dockerfile
# Stage bess-build: builds bess with its dependencies
FROM nefelinetworks/bess_build AS bess-build
RUN apt-get update && \
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

# linux ver should match target machine's kernel
ARG LINUX_VER=5.4.30
RUN wget -qO linux.tar.xz https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-${LINUX_VER}.tar.xz
RUN mkdir linux && \
    tar -xf linux.tar.xz -C linux --strip-components 1 && \
    cp linux/include/uapi/linux/if_xdp.h /usr/include/linux && \
    cd linux/tools/lib/bpf/ && \
    make $MAKEFLAGS install_lib && \
    make $MAKEFLAGS install_headers && \
    ldconfig

# dpdk
ARG DPDK_URL='http://dpdk.org/git/dpdk-stable'
ARG DPDK_VER='19.11'
ENV DPDK_DIR="/dpdk"
RUN git clone -b $DPDK_VER -q --depth 1 $DPDK_URL $DPDK_DIR

# Customizing DPDK install
WORKDIR $DPDK_DIR
COPY patches/dpdk patches
RUN cat patches/* | patch -p1

ARG CPU=native
ARG RTE_TARGET='x86_64-native-linuxapp-gcc'
RUN sed -ri 's,(IGB_UIO=).*,\1n,' config/common_linux* && \
    sed -ri 's,(KNI_KMOD=).*,\1n,' config/common_linux* && \
    sed -ri 's,(LIBRTE_BPF=).*,\1n,' config/common_base && \
    sed -ri 's,(LIBRTE_PMD_PCAP=).*,\1y,' config/common_base && \
    sed -ri 's,(PORT_PCAP=).*,\1y,' config/common_base && \
    sed -ri 's,(AF_XDP=).*,\1y,' config/common_base && \
    make config T=$RTE_TARGET && \
    make $MAKEFLAGS EXTRA_CFLAGS="-march=$CPU -g -w -fPIC -DALLOW_EXPERIMENTAL_API"

WORKDIR /
ARG BESS_COMMIT=master
RUN apt-get update && apt-get install -y wget unzip ca-certificates git
RUN wget -qO bess.zip https://github.com/NetSys/bess/archive/${BESS_COMMIT}.zip && unzip bess.zip
WORKDIR bess-${BESS_COMMIT}
COPY core/ core/
COPY patches/bess patches
COPY protobuf/ protobuf/
RUN cp -a ${DPDK_DIR} deps/dpdk-19.11.1 && \
    cat patches/* | patch -p1
RUN ./build.py --plugin sample_plugin bess && \
    cp bin/bessd /bin && \
    mkdir -p /bin/modules && \
    cp core/modules/*.so /bin/modules && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -r core/pb /pb && \
    cp -a protobuf /protobuf

# Stage pip: compile psutil
FROM python:2.7-slim AS pip
RUN apt-get update && apt-get install -y gcc
RUN pip install --no-cache-dir psutil

# Stage bess: creates the runtime image of BESS
FROM python:2.7-slim AS bess
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
        scapy
COPY --from=pip /usr/local/lib/python2.7/site-packages/psutil /usr/local/lib/python2.7/site-packages/psutil
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin/bessd
COPY --from=bess-build /bin/modules /bin/modules
COPY conf /opt/bess/bessctl/conf
RUN ln -s /opt/bess/bessctl/bessctl /bin
ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

FROM nefelinetworks/bess_build  AS cpiface-build
ARG MAKEFLAGS
ARG CPU=native
RUN apt-get update -y && apt-get install -y libzmq3-dev libjsoncpp-dev
WORKDIR /cpiface
COPY cpiface .
COPY --from=bess-build /pb pb
# Copying explicitly since symlinks don't work
COPY core/utils/gtp_common.h .
RUN make $MAKEFLAGS && \
    cp zmq-cpiface /bin

# Stage cpiface: creates runtime image of cpiface
FROM ubuntu:bionic AS cpiface
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgoogle-glog0v5 \
        libzmq5 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=cpiface-build /bin/zmq-cpiface /bin

FROM golang AS pfcpiface-build
WORKDIR /pfcpiface
COPY pfcpiface/go.mod pfcpiface/go.sum ./
RUN go mod download
COPY pfcpiface .
RUN CGO_ENABLED=0 go build -o /bin/pfcpiface

# Stage pfcpiface: runtime image of pfcpiface toward SMF/SPGW-C
FROM alpine AS pfcpiface
COPY --from=pfcpiface-build /bin/pfcpiface /bin
# Converting entrypoint from /bin/pfcpiface to /bin/sh for the time being
# The BESS pipeline is not installed @ dockerized init time.
ENTRYPOINT [ "/bin/pfcpiface" ]

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY --from=bess /bin/bessd /
COPY --from=cpiface /bin/zmq-cpiface /
COPY --from=pfcpiface /bin/pfcpiface /
COPY --from=bess-build /protobuf /protobuf
