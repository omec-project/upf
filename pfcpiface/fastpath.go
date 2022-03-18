// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"net"

	"github.com/prometheus/client_golang/prometheus"
)

type upfMsgType int

const (
	upfMsgTypeAdd upfMsgType = iota
	upfMsgTypeMod
	upfMsgTypeDel
	upfMsgTypeClear
)

func (u upfMsgType) String() string {
	if u == upfMsgTypeAdd {
		return "add"
	} else if u == upfMsgTypeMod {
		return "modify"
	} else if u == upfMsgTypeDel {
		return "delete" //nolint
	} else if u == upfMsgTypeClear {
		return "clear"
	} else {
		return "unknown"
	}
}

type fastPath interface {
	/* Close any pending sessions */
	exit()
	/* setup internal parameters and channel with fastPath */
	setUpfInfo(u *upf, conf *Conf)
	/* set up slice info */
	addSliceInfo(sliceInfo *SliceInfo) error
	/* write endMarker to fastpath */
	sendEndMarkers(endMarkerList *[][]byte) error
	/* write pdr/far/qer to fastpath */
	// "master" function to send create/update/delete messages to UPF.
	// "new" PacketForwardingRules are only used for update messages to UPF.
	// TODO: we should have better CRUD API, with a single function per message type.
	sendMsgToUPF(method upfMsgType, all PacketForwardingRules, new PacketForwardingRules) uint8
	/* check of communication channel to fastpath is setup */
	isConnected(accessIP *net.IP) bool
	summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric)
	portStats(uc *upfCollector, ch chan<- prometheus.Metric)
	sessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) error
}
