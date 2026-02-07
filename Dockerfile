# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright 2019-present Intel Corporation

# Stage bess-build: fetch BESS dependencies & pre-reqs
FROM ghcr.io/omec-project/bess_build:260206@sha256:3355d990fde583ad8d7eed5ae0c9200328d20dd91f7e6ecdf110a9beab48ffa7 AS bess-build
ARG CPU=native
ARG BESS_COMMIT=main
ENV PLUGINS_DIR=plugins
ARG MAKEFLAGS

# BESS pre-reqs
WORKDIR /bess
RUN git clone https://github.com/omec-project/bess.git . && \
    git checkout ${BESS_COMMIT} && \
    cp -a protobuf /protobuf

# Build DPDK
RUN ./build.py dpdk

# Plugins: SequentialUpdate
RUN mkdir -p plugins && \
    mv sample_plugin plugins

## Network Token
ARG ENABLE_NTF
ARG NTF_COMMIT=master
COPY scripts/install_ntf.sh .
RUN ./install_ntf.sh

# Build and copy artifacts
RUN PLUGINS=$(find "$PLUGINS_DIR" -mindepth 1 -maxdepth 1 -type d) && \
    CMD="./build.py bess" && \
    for PLUGIN in $PLUGINS; do \
        CMD="$CMD --plugin \"$PLUGIN\""; \
    done && \
    eval "$CMD" && \
    cp bin/bessd /bin && \
    mkdir -p /bin/modules && \
    cp core/modules/*.so /bin/modules && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -r core/pb /pb

# Stage bess: creates the runtime image of BESS
FROM ubuntu:22.04@sha256:104ae83764a5119017b8e8d6218fa0832b09df65aae7d5a6de29a85d813da2fb AS bess
WORKDIR /
COPY requirements.txt .
RUN apt-get update && apt-get install -y \
    --no-install-recommends \
    python3-pip \
    libgraph-easy-perl \
    iproute2 \
    iptables \
    iputils-ping \
    tcpdump && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    pip install --no-cache-dir --require-hashes -r requirements.txt
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin/bessd
COPY --from=bess-build /bin/modules /bin/modules
COPY conf /opt/bess/bessctl/conf
RUN ln -s /opt/bess/bessctl/bessctl /bin

# CNDP: Install dependencies
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y \
    --no-install-recommends \
    build-essential \
    ethtool \
    libbpf0 \
    libbsd0 \
    libc-ares2 \
    libelf1 \
    libgflags2.2 \
    libjson-c[45] \
    libnl-3-200 \
    libnl-cli-3-200 \
    libnuma1 \
    libpcap0.8 \
    libssl3 \
    pkg-config && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
COPY --from=bess-build /usr/bin/cndpfwd /usr/bin/
COPY --from=bess-build /usr/local/lib/x86_64-linux-gnu/*.so /usr/local/lib/x86_64-linux-gnu/
COPY --from=bess-build /usr/local/lib/x86_64-linux-gnu/*.a /usr/local/lib/x86_64-linux-gnu/
COPY --from=bess-build /usr/lib/libxdp* /usr/lib/
COPY --from=bess-build /usr/lib/x86_64-linux-gnu/libjson-c.so* /lib/x86_64-linux-gnu/
COPY --from=bess-build /usr/local/lib/libgrpc*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libgpr*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libre2*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libaddress_sorting*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libupb*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libutf8_range*.so* /usr/local/lib/
COPY --from=bess-build /usr/local/lib/libz.so* /usr/local/lib/
RUN ldconfig

ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

# Stage build bess golang pb
FROM golang:1.25.6-bookworm@sha256:2f768d462dbffbb0f0b3a5171009f162945b086f326e0b2a8fd5d29c3219ff14 AS protoc-gen
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

FROM bess-build AS go-pb
COPY --from=protoc-gen /go/bin/protoc-gen-go /bin
COPY --from=protoc-gen /go/bin/protoc-gen-go-grpc /bin

RUN mkdir /bess_pb && \
    protoc -I /usr/include -I /protobuf/ \
    /protobuf/*.proto /protobuf/ports/*.proto \
    --go_opt=paths=source_relative --go_out=/bess_pb \
    --go-grpc_opt=paths=source_relative --go-grpc_out=/bess_pb

FROM bess-build AS py-pb
COPY requirements_pb.txt .
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir --require-hashes -r requirements_pb.txt
RUN mkdir /bess_pb && \
    python3 -m grpc_tools.protoc -I /usr/include -I /protobuf/ \
    /protobuf/*.proto /protobuf/ports/*.proto \
    --python_out=plugins=grpc:/bess_pb \
    --grpc_python_out=/bess_pb

FROM golang:1.25.6-bookworm@sha256:2f768d462dbffbb0f0b3a5171009f162945b086f326e0b2a8fd5d29c3219ff14 AS pfcpiface-build
ARG GOFLAGS
WORKDIR /pfcpiface

COPY go.mod /pfcpiface/go.mod
COPY go.sum /pfcpiface/go.sum

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN if echo "$GOFLAGS" | grep -Eq "-mod=vendor"; then go mod download; fi

COPY . /pfcpiface
RUN go mod tidy && \
    CGO_ENABLED=0 go build $GOFLAGS -o /bin/pfcpiface ./cmd/pfcpiface

# Stage pfcpiface: runtime image of pfcpiface toward SMF/SPGW-C
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659 AS pfcpiface
COPY conf /opt/bess/bessctl/conf
COPY --from=pfcpiface-build /bin/pfcpiface /bin
ENTRYPOINT [ "/bin/pfcpiface" ]

# Stage pb: dummy stage for collecting protobufs
FROM scratch AS pb
COPY --from=bess-build /bess/protobuf /protobuf
COPY --from=go-pb /bess_pb /bess_pb

# Stage ptf-pb: dummy stage for collecting python protobufs
FROM scratch AS ptf-pb
COPY --from=bess-build /bess/protobuf /protobuf
COPY --from=py-pb /bess_pb /bess_pb

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY --from=bess /bin/bessd /
COPY --from=pfcpiface /bin/pfcpiface /
COPY --from=bess-build /bess /bess
