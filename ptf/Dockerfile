# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# Docker image to run PTF-based tests

ARG SCAPY_VER=2.4.5
ARG PTF_VER=c5299ea2e27386653209af458757b3b15e5dec5d
ARG TREX_VER=3b19ddcf67e33934f268b09d3364cd87275d48db
ARG TREX_EXT_LIBS=/external_libs
ARG TREX_LIBS=/trex_python
ARG UNITTEST_XML_REPORTING_VER=3.0.4
ARG PROTOBUF_VER=3.12
ARG GRPC_VER=1.26

# Install dependencies for general PTF test definitions
FROM ubuntu:20.04 as ptf-deps

ARG SCAPY_VER
ARG PTF_VER
ARG UNITTEST_XML_REPORTING_VER
ARG PROTOBUF_VER
ARG GRPC_VER

ENV RUNTIME_DEPS \
    python3 \
    python3-pip \
    python3-setuptools \
    git

ENV PIP_DEPS \
    git+https://github.com/p4lang/ptf@$PTF_VER \
    protobuf==$PROTOBUF_VER \
    grpcio==$GRPC_VER \
    unittest-xml-reporting==$UNITTEST_XML_REPORTING_VER

RUN apt update && \
    apt install -y $RUNTIME_DEPS
RUN pip3 install --no-cache-dir --root /python_output $PIP_DEPS

# Install TRex traffic gen and library for TRex API
FROM alpine:3.12.1 as trex-builder
ARG TREX_VER
ARG TREX_EXT_LIBS
ARG TREX_LIBS

ENV TREX_SCRIPT_DIR=/trex-core-${TREX_VER}/scripts

RUN wget https://github.com/stratum/trex-core/archive/${TREX_VER}.zip && \
    unzip -qq ${TREX_VER}.zip && \
    mkdir -p /output/${TREX_EXT_LIBS} && \
    mkdir -p /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/interactive/* /output/${TREX_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/external_libs/* /output/${TREX_EXT_LIBS} && \
    cp -r ${TREX_SCRIPT_DIR}/automation/trex_control_plane/stf/trex_stf_lib /output/${TREX_LIBS}

# Synthesize all dependencies for runtime
FROM ubuntu:20.04

ARG TREX_EXT_LIBS
ARG TREX_LIBS
ARG SCAPY_VER

ENV RUNTIME_DEPS \
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
    netbase

RUN apt update && \
    apt install -y $RUNTIME_DEPS && \
    rm -rf /var/lib/apt/lists/*
RUN pip3 install --no-cache-dir scipy==1.5.4 numpy==1.19.4 matplotlib==3.3.3 pyyaml==5.4.1 

ENV TREX_EXT_LIBS=${TREX_EXT_LIBS}
ENV PYTHONPATH=${TREX_EXT_LIBS}:${TREX_LIBS}

COPY --from=trex-builder /output /
COPY --from=ptf-deps /python_output /

# Install custom scapy version from TRex so PTF tests can access certain scapy features
RUN cd ${TREX_EXT_LIBS}/scapy-${SCAPY_VER}/ && python3 setup.py install
RUN ldconfig

ENTRYPOINT []
