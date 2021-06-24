// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"net"
)

type fastPath interface {
	/* Close any pending sessions */
	exit()
	/* setup internal parameters and channel with fastPath */
	setUpfInfo(u *upf, conf *Conf)
	/* set udp and pfcp conn parameters in interface */
	setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn)
	/* simulator mode setup */
	sim(u *upf, method string)
	/* write endMarker to fastpath */
	sendEndMarkers(endMarkerList *[][]byte) error
	/* write pdr/far/qer to fastpath */
	sendMsgToUPF(method string, pdrs []pdr, fars []far, qers []qer) uint8
	/* delete all pdrs/fars/qers/ installed in fastpath tabled */
	sendDeleteAllSessionsMsgtoUPF()
	/* check of communication channel to fastpath is setup */
	isConnected(accessIP *net.IP) bool
	summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric)
	portStats(uc *upfCollector, ch chan<- prometheus.Metric)
}
