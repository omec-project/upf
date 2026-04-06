# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright 2019-present Intel Corporation

# Stage bess-build: pre-built BESS image (built from bess/env/Dockerfile)
FROM ghcr.io/omec-project/bess_build:260403@sha256:e3c461647563ad4948e80e3e939006bae8bf2c83a8189be4e497e6135b04fa21 AS bess-build

# Stage bess: creates the runtime image of BESS
FROM ubuntu:24.04@sha256:d1e2e92c075e5ca139d51a140fff46f84315c0fdce203eab2807c7e495eff4f9 AS bess

ENV DEBIAN_FRONTEND=noninteractive

# Build arguments for dynamic labels
ARG VERSION=dev
ARG VCS_URL=unknown
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="${VCS_URL}" \
    org.opencontainers.image.version="${VERSION}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.revision="${VCS_REF}" \
    org.opencontainers.image.url="${VCS_URL}" \
    org.opencontainers.image.title="upf-bess" \
    org.opencontainers.image.description="Aether 5G Core UPF-BESS Network Function" \
    org.opencontainers.image.authors="Aether SD-Core <dev@lists.aetherproject.org>" \
    org.opencontainers.image.vendor="Aether Project" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.documentation="https://docs.sd-core.aetherproject.org/"

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
    pip install --no-cache-dir --break-system-packages --ignore-installed --require-hashes -r requirements.txt
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin/bessd
COPY --from=bess-build /bin/modules /bin/modules
COPY conf /opt/bess/bessctl/conf
RUN ln -s /opt/bess/bessctl/bessctl /bin

# CNDP and runtime: Install dependencies
RUN apt-get update && apt-get install -y \
    --no-install-recommends \
    build-essential \
    ethtool \
    libbpf1 \
    libbsd0 \
    libc-ares2 \
    libelf1 \
    libfdt1 \
    libgflags2.2 \
    libgoogle-glog0v6 \
    libgrpc++1.51t64 \
    libjson-c5 \
    libnl-3-200 \
    libnl-cli-3-200 \
    libnuma1 \
    libpcap0.8 \
    libprotobuf32t64 \
    libssl3 \
    libxdp1 \
    pkg-config && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
# Copy CNDP binary and libraries
COPY --from=bess-build /usr/bin/cndpfwd /usr/bin/
# NOTE: Copy the entire directory rather than individual library files because:
# - CNDP and DPDK install their runtime libraries into /usr/local/lib/x86_64-linux-gnu/
# - The exact set of required shared objects may change between CNDP/DPDK/BESS releases
# - Maintaining a fragile, version-specific list of libraries is error-prone
# - Image size impact has been evaluated and is acceptable for this component
COPY --from=bess-build /usr/local/lib/x86_64-linux-gnu/ /usr/local/lib/x86_64-linux-gnu/
# Create DPDK plugin directory so that EAL can dlopen bus/mempool/net drivers
# at runtime.  bessd passes "-d /opt/bess/lib/dpdk-pmds" to rte_eal_init()
# when this directory exists.
# Needed plugins:
#   librte_bus_vdev      – vdev bus (required to create AF_PACKET/AF_XDP ports)
#   librte_bus_pci       – PCI bus (required for DPDK-bound NICs)
#   librte_mempool_ring  – default "ring_mp_mc" mempool ops
#   librte_net_af_packet – AF_PACKET net driver (kernel datapath fallback)
#   librte_net_af_xdp    – AF_XDP net driver (kernel datapath)
RUN set -e; \
    mkdir -p /opt/bess/lib/dpdk-pmds; \
    missing_pats=""; \
    for pat in librte_mempool_ring librte_bus_vdev librte_bus_pci \
               librte_net_af_packet librte_net_af_xdp; do \
      found=0; \
      for f in /usr/local/lib/x86_64-linux-gnu/"${pat}".so*; do \
        if [ -f "$f" ]; then \
          ln -sf "$f" /opt/bess/lib/dpdk-pmds/; \
          found=1; \
        fi; \
      done; \
      if [ "$found" -eq 0 ]; then \
        echo "Required DPDK plugin not found: ${pat}" >&2; \
        missing_pats="yes"; \
      fi; \
    done; \
    if [ -n "$missing_pats" ]; then \
      echo "One or more required DPDK plugins are missing; failing build." >&2; \
      exit 1; \
    fi; \
    echo "DPDK PMD directory contents:"; \
    ls -la /opt/bess/lib/dpdk-pmds/; \
    ldconfig

ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

# Stage build bess golang pb
FROM golang:1.26.1-bookworm@sha256:8e8aa801e8417ef0b5c42b504dd34db3db911bb73dba933bd8bde75ed815fdbb AS protoc-gen
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
WORKDIR /
COPY requirements_pb.txt .
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir --break-system-packages --ignore-installed --require-hashes -r requirements_pb.txt
RUN mkdir /bess_pb && \
    python3 -m grpc_tools.protoc -I /usr/include -I /protobuf/ \
    /protobuf/*.proto /protobuf/ports/*.proto \
    --python_out=/bess_pb \
    --grpc_python_out=/bess_pb

FROM golang:1.26.1-bookworm@sha256:8e8aa801e8417ef0b5c42b504dd34db3db911bb73dba933bd8bde75ed815fdbb AS pfcpiface-build
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

# Build arguments for dynamic labels
ARG VERSION=dev
ARG VCS_URL=unknown
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="${VCS_URL}" \
    org.opencontainers.image.version="${VERSION}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.revision="${VCS_REF}" \
    org.opencontainers.image.url="${VCS_URL}" \
    org.opencontainers.image.title="pfcpiface" \
    org.opencontainers.image.description="Aether 5G Core PFCPIFACE Network Function" \
    org.opencontainers.image.authors="Aether SD-Core <dev@lists.aetherproject.org>" \
    org.opencontainers.image.vendor="Aether Project" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.documentation="https://docs.sd-core.aetherproject.org/"

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
COPY --from=bess-build /bess/protobuf /bess/protobuf
COPY --from=bess-build /pb /bess/pb
