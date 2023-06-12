#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2020 Intel Corporation
#
# Usage: reset_upf.sh cndp|dpdk true|false
# Currently this script only supports CNDP and DPDK modes.

MODE=${1:-cndp}

BUSY_POLL=${2:-true}

ACCESS_PCIE=0000:86:00.0
CORE_PCIE=0000:88:00.0

ACCESS_IFACE=enp134s0
CORE_IFACE=enp136s0

SET_IRQ_AFFINITY=~/nic/driver/ice-1.9.11/scripts/set_irq_affinity

sudo dpdk-devbind.py -u $ACCESS_PCIE --force
sudo dpdk-devbind.py -u $CORE_PCIE --force

sleep 2
echo "Stop UPF docker containers"
docker stop pause bess bess-routectl bess-web bess-pfcpiface || true
docker rm -f pause bess bess-routectl bess-web bess-pfcpiface || true

if [ "$MODE" == 'cndp' ]; then
	echo "Bind access/core interface to ICE driver"
	sudo dpdk-devbind.py -b ice $ACCESS_PCIE
	sudo dpdk-devbind.py -b ice $CORE_PCIE
	sudo ifconfig $ACCESS_IFACE up
	sudo ifconfig $CORE_IFACE up
	sudo systemctl disable --now irqbalance
	sudo $SET_IRQ_AFFINITY all $ACCESS_IFACE $CORE_IFACE

else
	echo "Bind access/core interface to DPDK"
	sudo dpdk-devbind.py -b vfio-pci $ACCESS_PCIE
	sudo dpdk-devbind.py -b vfio-pci $CORE_PCIE
fi

sleep 2

if [[ "$MODE" == 'cndp' ]] && [[ "$BUSY_POLL" == 'true' ]]; then
	echo "Setup configuration for XDP socket Busy Polling"

	# Refer: https://lwn.net/Articles/836250/
	echo 10 | sudo tee /sys/class/net/$ACCESS_IFACE/napi_defer_hard_irqs
	echo 2000000 | sudo tee /sys/class/net/$ACCESS_IFACE/gro_flush_timeout

	echo 10 | sudo tee /sys/class/net/$CORE_IFACE/napi_defer_hard_irqs
	echo 2000000 | sudo tee /sys/class/net/$CORE_IFACE/gro_flush_timeout
fi
