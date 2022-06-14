# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright 2019 Intel Corporation

# Multi-stage Dockerfile

# Stage bess-deps: fetch BESS dependencies
FROM ghcr.io/omec-project/upf-epc/bess_build AS bess-deps
# BESS pre-reqs
WORKDIR /bess
ARG BESS_COMMIT=dpdk-2011-focal
RUN curl -L https://github.com/NetSys/bess/tarball/${BESS_COMMIT} | \
    tar xz -C . --strip-components=1
COPY patches/bess patches
RUN cat patches/* | patch -p1
RUN cp -a protobuf /protobuf

# Stage bess-build: builds bess with its dependencies
FROM bess-deps AS bess-build
ARG CPU=native
RUN apt-get update && \
    apt-get -y install --no-install-recommends \
        ca-certificates \
        libelf-dev

ARG MAKEFLAGS

ENV PKG_CONFIG_PATH=/usr/lib64/pkgconfig

# linux ver should match target machine's kernel
WORKDIR /libbpf
ARG LIBBPF_VER=v0.3
RUN curl -L https://github.com/libbpf/libbpf/tarball/${LIBBPF_VER} | \
    tar xz -C . --strip-components=1 && \
    cp include/uapi/linux/if_xdp.h /usr/include/linux && \
    cd src && \
    make install && \
    ldconfig

WORKDIR /bess

# Patch BESS, patch and build DPDK
COPY patches/dpdk/* deps/
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
COPY core/ core/
COPY scripts/build_bess.sh .
RUN ./build_bess.sh && \
    cp bin/bessd /bin && \
    mkdir -p /bin/modules && \
    cp core/modules/*.so /bin/modules && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -r core/pb /pb

# Stage bess: creates the runtime image of BESS
FROM python:3.9.12-slim AS bess
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
