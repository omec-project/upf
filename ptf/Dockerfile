# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# Docker image to run PTF-based tests

# Cisco TRex v3.06 is commit 46be64bedbe9d505dfb55099c38119d6e86267ca.
ARG TREX_VER=3.06
ARG TREX_REPO=cisco-system-traffic-generator/trex-core
ARG TREX_ARCHIVE_SHA256=869c9120a427c507e023a12716ffd0332232a2c8c13e86b63d46a3e15d02b509
ARG TREX_APPLY_UPF_PATCHES=1
ARG TREX_EXT_LIBS=/external_libs
ARG TREX_LIBS=/trex_python
# Install dependencies for general PTF test definitions
FROM ubuntu:26.04@sha256:da6fc2be547864451aa253836dd926da33623312df4a9a243e35dc877c378a78 AS ptf-deps

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    python3 \
    python3-dev \
    python3-venv \
    git && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY requirements-ptf.txt /tmp/requirements-ptf.txt

RUN python3 -m venv /opt/ptf-venv && \
    /opt/ptf-venv/bin/pip install --no-cache-dir --require-hashes -r /tmp/requirements-ptf.txt && \
    /opt/ptf-venv/bin/pip check

# Install TRex traffic gen and library for TRex API
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS trex-builder
ARG TREX_VER
ARG TREX_REPO
ARG TREX_ARCHIVE_SHA256
ARG TREX_APPLY_UPF_PATCHES
ARG TREX_EXT_LIBS
ARG TREX_LIBS

ENV TREX_SCRIPT_DIR=/trex-core-${TREX_VER}/scripts

COPY patches/ /patches/

RUN apk add --no-cache patch wget unzip && \
    wget -q -O trex.zip https://github.com/${TREX_REPO}/archive/v${TREX_VER}.zip && \
    echo "${TREX_ARCHIVE_SHA256}  trex.zip" | sha256sum -c - && \
    unzip -qq trex.zip && \
    if [ "${TREX_APPLY_UPF_PATCHES}" = 1 ]; then \
        for trex_patch in /patches/trex/*.patch; do \
            patch -d /trex-core-${TREX_VER} -p1 < "${trex_patch}"; \
        done; \
    fi && \
    mkdir -p /output/${TREX_EXT_LIBS} && \
    mkdir -p /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/interactive/trex /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/interactive/trex_stl_lib /output/${TREX_LIBS} && \
    for trex_ext_lib in dpkt-1.9.1 pyyaml-3.11 pyzmq-ctypes repoze simpy-3.0.10 texttable-0.8.4 trex-openssl; do \
        cp -r ${TREX_SCRIPT_DIR}/external_libs/${trex_ext_lib} /output/${TREX_EXT_LIBS}; \
    done && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/stf/trex_stf_lib /output/${TREX_LIBS} && \
    test ! -e /output/${TREX_EXT_LIBS}/scapy-2.4.3

# Synthesize all dependencies for runtime
FROM ubuntu:26.04@sha256:da6fc2be547864451aa253836dd926da33623312df4a9a243e35dc877c378a78 AS ptf-runtime-deps

ARG TREX_EXT_LIBS
ARG TREX_LIBS

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    make \
    net-tools \
    ca-certificates \
    python3 \
    iproute2 \
    tcpdump \
    dumb-init \
    python3-dev \
    build-essential \
    wget \
    netbase && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY requirements-trex.txt /tmp/requirements-trex.txt

COPY --from=ptf-deps /opt/ptf-venv /opt/ptf-venv

RUN /opt/ptf-venv/bin/pip install --no-cache-dir --require-hashes -r /tmp/requirements-trex.txt && \
    /opt/ptf-venv/bin/pip check

ENV TREX_EXT_LIBS=${TREX_EXT_LIBS}
ENV PYTHONPATH=${TREX_EXT_LIBS}:${TREX_LIBS}
ENV PATH="/opt/ptf-venv/bin:${PATH}"

COPY --from=trex-builder /output /

FROM ptf-runtime-deps AS ptf-runtime

ENTRYPOINT []
