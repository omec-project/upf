#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2020 Intel Corporation

ENABLE_NTF=${ENABLE_NTF:-""}
NTF_COMMIT=${NTF_COMMIT:-master}
CJOSE_COMMIT=${CJOSE_COMMIT:-9261231f08d2a3cbcf5d73c5f9e754a2f1c379ac}
PLUGINS_DIR=${PLUGINS_DIR:-"plugins"}

SUDO=''
[[ $EUID -ne 0 ]] && SUDO=sudo

install_ntf() {
	set -e

	$SUDO apt install -y \
		autoconf \
		automake \
		clang-format \
		doxygen \
		libjansson-dev \
		libjansson4 \
		libtool

	pushd /
	wget -qO cjose.zip "https://github.com/cisco/cjose/archive/${CJOSE_COMMIT}.zip"
	unzip cjose.zip
	rm cjose.zip
	cd cjose-${CJOSE_COMMIT}
	./configure --prefix=/usr
	make
	make install
	popd

	wget -qO ntf.zip "https://github.com/Network-Tokens/ntf/archive/${NTF_COMMIT}.zip"
	unzip ntf.zip
	rm ntf.zip
	mv "ntf-${NTF_COMMIT}" "${PLUGINS_DIR}"
}

cleanup_image() {
	$SUDO rm -rf /var/lib/apt/lists/*
	$SUDO apt clean
}

(return 2>/dev/null) && echo "Sourced" && return

set -o errexit
set -o pipefail
set -o nounset

[ -z "$ENABLE_NTF" ] && exit 0

echo "Installing NTF plugin..."
install_ntf

echo "Cleaning up..."
cleanup_image
