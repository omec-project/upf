# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# Docker image to run PTF-based tests

ARG SCAPY_VER=2.4.5
ARG TREX_VER=2.92-scapy-2.4.5
ARG TREX_EXT_LIBS=/external_libs
ARG TREX_LIBS=/trex_python
# Install dependencies for general PTF test definitions
FROM ubuntu:26.04@sha256:2260313b31c8c011cd2eebe728008efac1b3982be73eb71348ea2648d2c0e09b AS ptf-deps

ARG SCAPY_VER

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
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS trex-builder
ARG TREX_VER
ARG TREX_EXT_LIBS
ARG TREX_LIBS

ENV TREX_SCRIPT_DIR=/trex-core-${TREX_VER}/scripts

RUN apk add --no-cache wget unzip && \
    wget https://github.com/stratum/trex-core/archive/v${TREX_VER}.zip && \
    unzip -qq v${TREX_VER}.zip && \
    mkdir -p /output/${TREX_EXT_LIBS} && \
    mkdir -p /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/interactive/* /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/external_libs/* /output/${TREX_EXT_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/stf/trex_stf_lib /output/${TREX_LIBS}

# Synthesize all dependencies for runtime
FROM ubuntu:26.04@sha256:2260313b31c8c011cd2eebe728008efac1b3982be73eb71348ea2648d2c0e09b

ARG TREX_EXT_LIBS
ARG TREX_LIBS
ARG SCAPY_VER

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

# Install TRex's custom Scapy so PTF tests can access its required features.
# This intentionally replaces the checked Scapy 2.7.0 dependency. PTF 0.12.0
# declares Scapy >=2.5.0, so a final pip check would report this known legacy
# incompatibility; validate the runtime imports and intended version instead.
# TODO: Move to a newer Scapy version compatible with the PTF requirement.
WORKDIR ${TREX_EXT_LIBS}/scapy-${SCAPY_VER}
RUN printf '%s\n' "${SCAPY_VER}" > scapy/VERSION && \
    /opt/ptf-venv/bin/python -m pip wheel --no-cache-dir --no-build-isolation --no-deps --wheel-dir /tmp/wheels . && \
    /opt/ptf-venv/bin/python -m pip install --no-cache-dir --no-deps /tmp/wheels/scapy-*.whl && \
    /opt/ptf-venv/bin/python -c "from importlib.metadata import version; import ptf, scapy; assert version('scapy') == '${SCAPY_VER}'" && \
    ldconfig

ENTRYPOINT []
