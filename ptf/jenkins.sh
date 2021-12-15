#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2021 Open Networking Foundation

make build && TREX_PARAMS="--trex-sw-mode" ./run_tests -t tests/linerate
