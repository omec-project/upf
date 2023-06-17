# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright 2019 Intel Corporation

# Multi-stage Dockerfile

# Stage bess-deps: fetch BESS dependencies
FROM ghcr.io/omec-project/upf-epc/bess_build AS bess-deps
RUN apt-get update && \
    apt-get install -y git

# BESS pre-reqs
WORKDIR /bess
ARG BESS_COMMIT=dpdk-2011-focal
RUN git clone https://github.com/omec-project/bess.git .
RUN git checkout ${BESS_COMMIT}
RUN cp -a protobuf /protobuf

# Stage bess-build: builds bess with its dependencies
FROM bess-deps AS bess-build
ARG CPU=native
RUN apt-get update && \
    apt-get -y install --no-install-recommends \
        ca-certificates \
        libelf-dev \
        libbpf0

ARG MAKEFLAGS
ENV PKG_CONFIG_PATH=/usr/lib64/pkgconfig
WORKDIR /bess

# Patch and build DPDK
RUN ./build.py dpdk

# Plugins
RUN mkdir -p plugins

## SequentialUpdate
RUN mv sample_plugin plugins

## Network Token
ARG ENABLE_NTF
ARG NTF_COMMIT=master
COPY scripts/install_ntf.sh .
RUN ./install_ntf.sh

# Build and copy artifacts
COPY scripts/build_bess.sh .
RUN ./build_bess.sh && \
    cp bin/bessd /bin && \
    mkdir -p /bin/modules && \
    cp core/modules/*.so /bin/modules && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -r core/pb /pb

# Stage bess: creates the runtime image of BESS
FROM python:3.11.3-slim AS bess
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        gcc \
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
        mitogen \
        protobuf==3.20.0 \
        psutil \
        pyroute2 \
        scapy && \
    apt-get --purge remove -y \
        gcc
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin/bessd
COPY --from=bess-build /bin/modules /bin/modules
COPY conf /opt/bess/bessctl/conf
RUN ln -s /opt/bess/bessctl/bessctl /bin

# CNDP: Install dependencies
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y \
    build-essential \
    ethtool \
    libbsd0 \
    libbpf0 \
    libelf1 \
    libgflags2.2 \
    libjson-c[45] \
    libnl-3-200 \
    libnl-cli-3-200 \
    libnuma1 \
    libpcap0.8 \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*
COPY --from=bess-build /usr/bin/cndpfwd /usr/bin/
COPY --from=bess-build /usr/local/lib/x86_64-linux-gnu/*.so /usr/local/lib/x86_64-linux-gnu/
COPY --from=bess-build /usr/local/lib/x86_64-linux-gnu/*.a /usr/local/lib/x86_64-linux-gnu/
COPY --from=bess-build /usr/lib/libxdp* /usr/lib/
COPY --from=bess-build /lib/x86_64-linux-gnu/libjson-c.so* /lib/x86_64-linux-gnu/

ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

# Stage build bess golang pb
FROM golang AS protoc-gen
RUN go install github.com/golang/protobuf/protoc-gen-go@latest

FROM bess-deps AS go-pb
COPY --from=protoc-gen /go/bin/protoc-gen-go /bin
RUN mkdir /bess_pb && \
    protoc -I /usr/include -I /protobuf/ \
        /protobuf/*.proto /protobuf/ports/*.proto \
        --go_opt=paths=source_relative --go_out=plugins=grpc:/bess_pb

FROM bess-deps AS py-pb
RUN pip install grpcio-tools==1.26
RUN mkdir /bess_pb && \
    python -m grpc_tools.protoc -I /usr/include -I /protobuf/ \
        /protobuf/*.proto /protobuf/ports/*.proto \
        --python_out=plugins=grpc:/bess_pb \
        --grpc_python_out=/bess_pb

FROM golang AS pfcpiface-build
ARG GOFLAGS
WORKDIR /pfcpiface

COPY go.mod /pfcpiface/go.mod
COPY go.sum /pfcpiface/go.sum

RUN if [[ ! "$GOFLAGS" =~ "-mod=vendor" ]] ; then go mod download ; fi

COPY . /pfcpiface
RUN CGO_ENABLED=0 go build $GOFLAGS -o /bin/pfcpiface ./cmd/pfcpiface

# Stage pfcpiface: runtime image of pfcpiface toward SMF/SPGW-C
FROM alpine AS pfcpiface
COPY conf /opt/bess/bessctl/conf
COPY --from=pfcpiface-build /bin/pfcpiface /bin
ENTRYPOINT [ "/bin/pfcpiface" ]

# Stage pb: dummy stage for collecting protobufs
FROM scratch AS pb
COPY --from=bess-deps /bess/protobuf /protobuf
COPY --from=go-pb /bess_pb /bess_pb

# Stage ptf-pb: dummy stage for collecting python protobufs
FROM scratch AS ptf-pb
COPY --from=bess-deps /bess/protobuf /protobuf
COPY --from=py-pb /bess_pb /bess_pb

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY --from=bess /bin/bessd /
COPY --from=pfcpiface /bin/pfcpiface /
COPY --from=bess-build /bess /bess
