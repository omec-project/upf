#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2020 Intel Corporation

ENABLE_NTF=${ENABLE_NTF:-0}
NTF_COMMIT=${NTF_COMMIT:-master}
CJOSE_COMMIT=${CJOSE_COMMIT:-9261231f08d2a3cbcf5d73c5f9e754a2f1c379ac}
PLUGINS_DIR=${PLUGINS_DIR:-"plugins"}
PLUGIN_DEST_DIR="${PLUGINS_DIR}/ntf"

SUDO=''
[[ $EUID -ne 0 ]] && SUDO=sudo

install_ntf() {
	$SUDO apt install -y \
		autoconf \
		automake \
		clang-format \
		doxygen \
		libjansson-dev \
		libjansson4 \
		libtool

	mkdir /tmp/cjose
	pushd /tmp/cjose
	curl -L "https://github.com/cisco/cjose/tarball/${CJOSE_COMMIT}" |
		tar xz -C . --strip-components=1
	./configure --prefix=/usr
	make
	make install
	popd

	mkdir ${PLUGIN_DEST_DIR}
	pushd ${PLUGIN_DEST_DIR}
	curl -L "https://github.com/Network-Tokens/ntf/tarball/${NTF_COMMIT}" |
		tar xz -C . --strip-components=1
	popd
}

cleanup_image() {
	$SUDO rm -rf /var/lib/apt/lists/*
	$SUDO apt clean
}

(return 2>/dev/null) && echo "Sourced" && return

set -o errexit
set -o pipefail
set -o nounset

[ "$ENABLE_NTF" == "0" ] && exit 0

echo "Installing NTF plugin..."
install_ntf

echo "Cleaning up..."
cleanup_image
