// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"net"
)

type fastPath interface {
	exit()
	setUpfInfo(u *upf, conf *Conf)
	setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn)
	sim(u *upf, method string)
	sendMsgToUPF(method string, pdrs []pdr, fars []far) uint8
	sendDeleteAllSessionsMsgtoUPF()
	summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric)
	portStats(uc *upfCollector, ch chan<- prometheus.Metric)
}
