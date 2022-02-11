#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2020 Intel Corporation

PLUGINS_DIR=${PLUGINS_DIR:-"plugins"}

build_bess() {
	PLUGINS=$(find "$PLUGINS_DIR" -mindepth 1 -maxdepth 1 -type d)
	echo "Found plugins: $PLUGINS"

	CMD="./build.py bess"
	for PLUGIN in $PLUGINS; do
		CMD="$CMD --plugin $PLUGIN"
	done
	eval $CMD
}

(return 2>/dev/null) && echo "Sourced" && return

set -o errexit
set -o pipefail
set -o nounset

echo "Building BESS..."
build_bess
