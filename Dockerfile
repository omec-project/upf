# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright (c) 2019 Intel Corporation

# Multi-stage Dockerfile
# Stage bess-build: builds bess with its dependencies
FROM nefelinetworks/bess_build AS bess-build
RUN apt-get update && \
    apt-get -y install --no-install-recommends \
        ca-certificates \
        libelf-dev

ARG MAKEFLAGS

# linux ver should match target machine's kernel
WORKDIR /libbpf
ARG LIBBPF_VER=v0.0.9
RUN curl -L https://github.com/libbpf/libbpf/tarball/${LIBBPF_VER} | \
    tar xz -C . --strip-components=1 && \
    cp include/uapi/linux/if_xdp.h /usr/include/linux && \
    cd src && \
    make install && \
    ldconfig

# BESS pre-reqs
WORKDIR /bess
ARG BESS_COMMIT=master
RUN curl -L https://github.com/NetSys/bess/tarball/${BESS_COMMIT} | \
    tar xz -C . --strip-components=1

# Patch BESS, patch and build DPDK
COPY patches/dpdk/* deps/
COPY patches/bess patches
RUN cat patches/* | patch -p1 && \
    ./build.py dpdk

# Hack to get static version linked
RUN rm -f /usr/lib64/libbpf.so*

# Plugins
RUN mkdir -p plugins

## SequentialUpdate
RUN mv sample_plugin plugins

## Network Token
ARG ENABLE_NTF
ARG NTF_COMMIT=master
COPY install_ntf.sh .
RUN ./install_ntf.sh

# Build and copy artifacts
COPY core/ core/
COPY protobuf/ protobuf/
COPY build_bess.sh .
RUN ./build_bess.sh && \
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
ENTRYPOINT [ "/bin/pfcpiface" ]

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY --from=bess /bin/bessd /
COPY --from=cpiface /bin/zmq-cpiface /
COPY --from=pfcpiface /bin/pfcpiface /
COPY --from=bess-build /protobuf /protobuf
