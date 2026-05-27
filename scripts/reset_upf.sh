#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2020 Intel Corporation
#
# Usage: reset_upf.sh
# This script resets the UPF back to DPDK mode.

MODE=dpdk

ACCESS_PCIE=0000:86:00.0
CORE_PCIE=0000:88:00.0

ACCESS_IFACE=enp134s0
CORE_IFACE=enp136s0

SET_IRQ_AFFINITY=~/nic/driver/ice-1.9.11/scripts/set_irq_affinity

sudo dpdk-devbind.py -u $ACCESS_PCIE --force
sudo dpdk-devbind.py -u $CORE_PCIE --force

sleep 2
echo "Stop UPF docker containers"
docker stop pause bess bess-routectl bess-web bess-pfcp || true
docker rm -f pause bess bess-routectl bess-web bess-pfcp || true

echo "Bind access/core interface to DPDK"
sudo dpdk-devbind.py -b vfio-pci $ACCESS_PCIE
sudo dpdk-devbind.py -b vfio-pci $CORE_PCIE

sleep 2
