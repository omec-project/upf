// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"github.com/prometheus/client_golang/prometheus"
	"net"
)

type upfMsgType int

const (
	upfMsgTypeAdd upfMsgType = iota
	upfMsgTypeMod
	upfMsgTypeDel
	upfMsgTypeClear
)

type FastpathRuntimeConfig struct {

}

type fastPath interface {
	// Connect setup internal parameters and channel with fastPath.
	Connect() (accessIP net.IP, coreIP net.IP, err error)
	// Clear deletes the entire fastpath state.
	Clear()
	// IsConnected checks whether communication channel to fastpath is setup.
	IsConnected() bool

	// Create installs PFCP Create IEs into fastpath.
	// The 'session' argument gives fastpath access to all PFCP rules created so far.
	// The 'created' argument only contains PFCP rules created in a given PFCP message.
	Create(session *PFCPSession, created PacketForwardingRules) error
	// Modify installs PFCP Modify IEs into fastpath.
	// The 'session' argument gives fastpath access to all PFCP rules created so far.
	// The 'new' argument only contains PFCP rules modified in a given PFCP message.
	Modify(session *PFCPSession, new PacketForwardingRules) error
	// Remove installs individual PFCP Remove IEs into fastpath.
	// The 'session' argument gives fastpath access to all PFCP rules created so far.
	// The 'toDelete' argument only contains PFCP rules that should be deleted by a given PFCP message.
	Remove(session *PFCPSession, toDelete PacketForwardingRules) error
	// RemoveAll deletes all the PFCP rules created so far from a fastpath.
	RemoveAll(session *PFCPSession) error

	// SendEndMarkers writes endMarker to fastpath
	SendEndMarkers(endMarkerList *[][]byte) error

	// Exit close channel to fastpath.
	Exit()


	/* setup internal parameters and channel with fastPath */
	setUpfInfo(u *upf, conf *Conf)
	/* set up slice info */
	SetSliceConfig(sliceInfo *SliceInfo) error
	GetLatencyJitterStats(uc *upfCollector, ch chan<- prometheus.Metric)
	GetPortStats(uc *upfCollector, ch chan<- prometheus.Metric)
	GetSessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) error
}
