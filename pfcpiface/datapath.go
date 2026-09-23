// SPDX-License-Identifier: Apache-2.0
// Copyright 2020-present Intel Corporation

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

const (
	upfMethodAdd    = "add"
	upfMethodModify = "modify"
	upfMethodDelete = "delete"
	upfMethodClear  = "clear"
)

func (u upfMsgType) String() string {
	switch u {
	case upfMsgTypeAdd:
		return upfMethodAdd
	case upfMsgTypeMod:
		return upfMethodModify
	case upfMsgTypeDel:
		return upfMethodDelete //nolint
	case upfMsgTypeClear:
		return upfMethodClear
	default:
		return UnknownString
	}
}

type datapath interface {
	/* Close any pending sessions */
	Exit()
	/* setup internal parameters and channel with datapath */
	SetUpfInfo(u *upf, conf *Conf)
	/* set up slice info */
	AddSliceInfo(sliceInfo *SliceInfo) error
	/* write endMarker to datapath */
	SendEndMarkers(endMarkerList *[][]byte) error
	/* write pdr/far/qer to datapath */
	// "master" function to send create/update/delete messages to UPF.
	// "newRules" PacketForwardingRules are only used for update messages to UPF.
	// TODO: we should have better CRUD API, with a single function per message type.
	SendMsgToUPF(method upfMsgType, all PacketForwardingRules, newRules PacketForwardingRules) uint8
	// SendMsgToUPFWithCompletion is SendMsgToUPF with the other half of the answer:
	// whether the batch finished. The cause alone answers a batch that ran out of time as
	// accepted, which is right for a caller that forgets a session on a rejection and
	// wrong for one that forgets it on an acceptance; a caller of the second kind needs
	// to know which it was.
	SendMsgToUPFWithCompletion(
		method upfMsgType, all PacketForwardingRules, newRules PacketForwardingRules,
	) (cause uint8, finished bool)
	/* check of communication channel to datapath is setup */
	IsConnected(accessIP *net.IP) bool
	SummaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric)
	PortStats(uc *upfCollector, ch chan<- prometheus.Metric)
	SummaryGtpuLatency(uc *upfCollector, ch chan<- prometheus.Metric)
	SessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) error
}
