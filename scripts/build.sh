#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

set -o errexit
set -o pipefail
set -o nounset

: "${MAKEFLAGS:=-j$(nproc)}"
: "${DOCKER_BUILDKIT:=1}"
export MAKEFLAGS
export DOCKER_BUILDKIT
docker build --pull --build-arg MAKEFLAGS --target=bess -t spgwu .
docker build --pull --build-arg MAKEFLAGS --target=cpiface -t cpiface .
