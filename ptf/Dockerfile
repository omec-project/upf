# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# Docker image to run PTF-based tests

ARG SCAPY_VER=2.4.5
ARG PTF_VER=c5299ea2e27386653209af458757b3b15e5dec5d
ARG TREX_VER=2.92-scapy-2.4.5
ARG TREX_EXT_LIBS=/external_libs
ARG TREX_LIBS=/trex_python
ARG UNITTEST_XML_REPORTING_VER=3.0.4
ARG PROTOBUF_VER=3.20
# Install dependencies for general PTF test definitions
FROM ubuntu:22.04 AS ptf-deps

ARG SCAPY_VER
ARG PTF_VER
ARG UNITTEST_XML_REPORTING_VER
ARG PROTOBUF_VER
ARG GRPC_VER

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-setuptools \
    git && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --root /python_output \
    git+https://github.com/p4lang/ptf@$PTF_VER \
    protobuf==$PROTOBUF_VER \
    grpcio \
    unittest-xml-reporting==$UNITTEST_XML_REPORTING_VER

# Install TRex traffic gen and library for TRex API
FROM alpine:3.20.3 AS trex-builder
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
FROM ubuntu:22.04

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
RUN pip3 install --no-cache-dir \
    scipy \
    numpy \
    matplotlib \
    pyyaml

ENV TREX_EXT_LIBS=${TREX_EXT_LIBS}
ENV PYTHONPATH=${TREX_EXT_LIBS}:${TREX_LIBS}

COPY --from=trex-builder /output /
COPY --from=ptf-deps /python_output /

# Install custom scapy version from TRex so PTF tests can access certain scapy features
# TODO: Move to a newer Scapy version compatible with latest Ubuntu versions
WORKDIR ${TREX_EXT_LIBS}/scapy-${SCAPY_VER}
RUN python3 setup.py install \
    && ldconfig

ENTRYPOINT []
