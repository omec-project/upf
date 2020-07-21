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

FROM golang AS pfcpiface-build
WORKDIR /pfcpiface
COPY pfcpiface/go.mod pfcpiface/go.sum ./
RUN go mod download
COPY pfcpiface .
RUN CGO_ENABLED=0 go build -o /bin/pfcpiface

# Stage pfcpiface: runtime image of pfcpiface toward SMF/SPGW-C
FROM alpine AS pfcpiface
COPY conf /opt/bess/bessctl/conf
COPY conf/p4info.bin /bin/
COPY conf/p4info.txt /bin/
COPY conf/bmv2.json /bin/
COPY --from=pfcpiface-build /bin/pfcpiface /bin
ENTRYPOINT [ "/bin/sh" ]

# Stage binaries: dummy stage for collecting artifacts
FROM scratch AS artifacts
COPY conf /opt/bess/bessctl/conf
COPY --from=pfcpiface /bin/pfcpiface /
