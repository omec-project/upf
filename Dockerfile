# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# Copyright 2019-present Intel Corporation

# Stage builder: install BESS build dependencies from the imported source tree
FROM ubuntu:26.04@sha256:f3d28607ddd78734bb7f71f117f3c6706c666b8b76cbff7c9ff6e5718d46ff64 AS builder

ENV DEBIAN_FRONTEND=noninteractive

COPY bess/env/ansible.cfg /tmp/
COPY bess/env/build-dep.yml /tmp/
COPY bess/env/kmod.yml /tmp/
COPY bess/env/ci.yml /tmp/
COPY bess/env/requirements-dev.txt /tmp/

RUN apt-get update && \
    apt-get install -y \
    --no-install-recommends \
    ansible \
    curl && \
    ANSIBLE_CONFIG=/tmp/ansible.cfg ansible-playbook /tmp/ci.yml -i "localhost," -c local && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Stage bess-build: build BESS from the imported source tree
FROM builder AS bess-build

ARG CPU=haswell
ARG MAKEFLAGS
ENV CPU=${CPU} MAKEFLAGS=${MAKEFLAGS} PLUGINS_DIR=plugins
ENV PKG_CONFIG_PATH=/usr/lib/pkgconfig:/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/local/lib/pkgconfig:/usr/local/lib/x86_64-linux-gnu/pkgconfig
ENV BESS_LINK_DYNAMIC=1

WORKDIR /bess
COPY bess .
RUN cp -a protobuf /protobuf && \
    mkdir -p plugins && \
    mv sample_plugin plugins

RUN PLUGINS=$(find "$PLUGINS_DIR" -mindepth 1 -maxdepth 1 -type d) && \
    CMD="./build.py bess" && \
    for PLUGIN in $PLUGINS; do \
        CMD="$CMD --plugin \"$PLUGIN\""; \
    done && \
    eval "$CMD" && \
    cp bin/bessd /bin && \
    strip /bin/bessd && \
    mkdir -p /bin/modules && \
    cp core/modules/*.so /bin/modules && \
    mkdir -p /opt/bess && \
    cp -r bessctl pybess /opt/bess && \
    cp -a core/pb /pb

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN mkdir -p /staging/lib && \
    for lib in $(ldd /bin/bessd 2>/dev/null | grep -E '/usr/(local/)?lib' | awk '{print $3}'); do \
      cp -aL "${lib}"* /staging/lib/ 2>/dev/null || true; \
    done && \
    cp -aL /usr/lib/x86_64-linux-gnu/librte_*.so* /staging/lib/ 2>/dev/null || true && \
    mkdir -p /staging/opt/bess/lib/dpdk-pmds && \
    missing_pats="" && \
    for pat in librte_mempool_ring librte_bus_vdev librte_bus_pci; do \
      found=0; \
      for f in /staging/lib/"${pat}".so*; do \
        if [ -f "$f" ]; then \
          ln -sf "/usr/local/lib/x86_64-linux-gnu/$(basename "$f")" \
            /staging/opt/bess/lib/dpdk-pmds/; \
          found=1; \
        fi; \
      done; \
      if [ "$found" -eq 0 ]; then \
        echo "Required DPDK plugin not found: ${pat}" >&2; \
        missing_pats="yes"; \
      fi; \
    done && \
    for pat in librte_net_af_packet librte_net_af_xdp; do \
      found=0; \
      for f in /staging/lib/"${pat}".so*; do \
        if [ -f "$f" ]; then \
          ln -sf "/usr/local/lib/x86_64-linux-gnu/$(basename "$f")" \
            /staging/opt/bess/lib/dpdk-pmds/; \
          found=1; \
        fi; \
      done; \
      if [ "$found" -eq 0 ]; then \
        echo "Required DPDK net PMD not found: ${pat}" >&2; \
        missing_pats="yes"; \
      fi; \
    done && \
    for pat in librte_net_bond librte_net_e1000 librte_net_i40e \
               librte_net_iavf librte_net_ice librte_net_igc \
               librte_net_ixgbe librte_net_idpf librte_net_cpfl; do \
      for f in /staging/lib/"${pat}".so*; do \
        if [ -f "$f" ]; then \
          ln -sf "/usr/local/lib/x86_64-linux-gnu/$(basename "$f")" \
            /staging/opt/bess/lib/dpdk-pmds/; \
        fi; \
      done; \
    done && \
    if [ -n "$missing_pats" ]; then \
      echo "One or more required DPDK plugins are missing; failing build." >&2; \
      exit 1; \
    fi && \
    echo "DPDK PMD staging directory contents:" && \
    ls -la /staging/opt/bess/lib/dpdk-pmds/

# Stage bess: creates the runtime image of BESS
FROM ubuntu:26.04@sha256:f3d28607ddd78734bb7f71f117f3c6706c666b8b76cbff7c9ff6e5718d46ff64 AS bess

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /
COPY requirements.txt .
COPY bess/env/ansible.cfg /tmp/
COPY bess/env/runtime-deps.yml /tmp/
RUN apt-get update && apt-get install -y \
    --no-install-recommends \
    ansible \
    iproute2 \
    iptables \
    iputils-ping && \
    ANSIBLE_CONFIG=/tmp/ansible.cfg ansible-playbook /tmp/runtime-deps.yml \
        -i "localhost," -c local --tags runtime-apt && \
    apt-get purge -y ansible && \
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/ansible.cfg /tmp/runtime-deps.yml && \
    pip install --no-cache-dir --break-system-packages --ignore-installed --require-hashes -r requirements.txt
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin/bessd
COPY --from=bess-build /bin/modules /bin/modules
COPY --from=bess-build /protobuf /protobuf
COPY --from=bess-build /protobuf /bess/protobuf
COPY --from=bess-build /usr/bin/protoc /usr/local/bin/
COPY --from=bess-build /usr/include/google/protobuf /usr/local/include/google/protobuf
COPY conf /opt/bess/bessctl/conf
RUN ln -s /opt/bess/bessctl/bessctl /bin

# UPF-specific runtime dependencies. BESS runtime packages are installed from
# bess/env/runtime-deps.yml above, which is also used by BESS host installs.
RUN apt-get update && apt-get install -y \
    --no-install-recommends \
    build-essential \
    pkg-config && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
# NOTE: Copy the entire directory rather than individual library files because:
# - BESS and DPDK install their runtime libraries into /usr/local/lib/x86_64-linux-gnu/
# - The exact set of required shared objects may change between DPDK/BESS releases
# - Maintaining a fragile, version-specific list of libraries is error-prone
# - Image size impact has been evaluated and is acceptable for this component
COPY --from=bess-build /staging/lib/ /usr/local/lib/x86_64-linux-gnu/
COPY --from=bess-build /staging/opt/bess/lib/dpdk-pmds/ /opt/bess/lib/dpdk-pmds/
# BESS owns the staged PMD selection.  The final image only copies the selected
# directory and refreshes the dynamic linker cache.
RUN echo "DPDK PMD directory contents:"; \
    ls -la /opt/bess/lib/dpdk-pmds/; \
    ldconfig

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

ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]

# Stage build bess golang pb
FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS protoc-gen
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

FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS pfcp-build
ARG GOFLAGS
WORKDIR /pfcpiface

COPY go.mod /pfcpiface/go.mod
COPY go.sum /pfcpiface/go.sum

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN if echo "$GOFLAGS" | grep -Eq "-mod=vendor"; then go mod download; fi

COPY . /pfcpiface
RUN go mod tidy && \
    CGO_ENABLED=0 go build $GOFLAGS -o /bin/pfcpiface ./cmd/pfcpiface

# Stage pfcp: runtime image of pfcp agent towards SMF
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS pfcp

COPY conf /opt/bess/bessctl/conf
COPY --from=pfcp-build /bin/pfcpiface /bin

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
    org.opencontainers.image.title="pfcp" \
    org.opencontainers.image.description="Aether 5G Core PFCP Agent for User Plane Function" \
    org.opencontainers.image.authors="Aether SD-Core <dev@lists.aetherproject.org>" \
    org.opencontainers.image.vendor="Aether Project" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.documentation="https://docs.sd-core.aetherproject.org/"

ENTRYPOINT [ "/bin/pfcpiface" ]

# Stage pb: dummy stage for collecting protobufs
FROM scratch AS pb
COPY --from=bess-build /protobuf /protobuf
COPY --from=go-pb /bess_pb /bess_pb

# Stage ptf-pb: dummy stage for collecting python protobufs
FROM scratch AS ptf-pb
COPY --from=bess-build /protobuf /protobuf
COPY --from=py-pb /bess_pb /bess_pb

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY --from=bess /bin/bessd /
COPY --from=pfcp /bin/pfcpiface /
COPY --from=bess-build /protobuf /bess/protobuf
COPY --from=bess-build /pb /bess/pb
