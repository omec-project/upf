#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2021 Open Networking Foundation

set -eux -o pipefail

make build
./run_tests -t tests/linerate
