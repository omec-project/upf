# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# Docker image to run PTF-based tests

ARG SCAPY_VER=2.4.5
ARG TREX_VER=2.92-scapy-2.4.5
ARG TREX_EXT_LIBS=/external_libs
ARG TREX_LIBS=/trex_python
# Install dependencies for general PTF test definitions
FROM ubuntu:24.04@sha256:d1e2e92c075e5ca139d51a140fff46f84315c0fdce203eab2807c7e495eff4f9 AS ptf-deps

ARG SCAPY_VER

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-setuptools \
    git && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY requirements-ptf.txt /tmp/requirements-ptf.txt

RUN pip3 install --no-cache-dir --break-system-packages --require-hashes --root /python_output -r /tmp/requirements-ptf.txt

# Install TRex traffic gen and library for TRex API
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659 AS trex-builder
ARG TREX_VER
ARG TREX_EXT_LIBS
ARG TREX_LIBS

ENV TREX_SCRIPT_DIR=/trex-core-${TREX_VER}/scripts

RUN apk update && apk add --no-cache -U wget && \
    wget https://github.com/stratum/trex-core/archive/v${TREX_VER}.zip && \
    unzip -qq v${TREX_VER}.zip && \
    mkdir -p /output/${TREX_EXT_LIBS} && \
    mkdir -p /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/interactive/* /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/external_libs/* /output/${TREX_EXT_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/stf/trex_stf_lib /output/${TREX_LIBS}

# Synthesize all dependencies for runtime
FROM ubuntu:24.04@sha256:d1e2e92c075e5ca139d51a140fff46f84315c0fdce203eab2807c7e495eff4f9

ARG TREX_EXT_LIBS
ARG TREX_LIBS
ARG SCAPY_VER

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    make \
    net-tools \
    python3 \
    python3-setuptools \
    iproute2 \
    tcpdump \
    dumb-init \
    python3-dev \
    build-essential \
    python3-pip \
    wget \
    netbase && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY requirements-trex.txt /tmp/requirements-trex.txt

RUN pip3 install --no-cache-dir --break-system-packages --require-hashes -r /tmp/requirements-trex.txt

ENV TREX_EXT_LIBS=${TREX_EXT_LIBS}
ENV PYTHONPATH=${TREX_EXT_LIBS}:${TREX_LIBS}

COPY --from=trex-builder /output /
COPY --from=ptf-deps /python_output /

# Install custom scapy version from TRex so PTF tests can access certain scapy features
# TODO: Move to a newer Scapy version compatible with latest Ubuntu versions
WORKDIR ${TREX_EXT_LIBS}/scapy-${SCAPY_VER}
RUN printf '%s\n' "${SCAPY_VER}" > scapy/VERSION && \
    python3 -m pip wheel --no-cache-dir --no-build-isolation --no-deps --wheel-dir /tmp/wheels . && \
    python3 -m pip install --no-cache-dir --break-system-packages /tmp/wheels/scapy-*.whl && \
    ldconfig

ENTRYPOINT []
