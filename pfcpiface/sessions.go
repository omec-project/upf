// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"fmt"
	"sync"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

type notifyFlag struct {
	flag bool
	mux  sync.Mutex
}

type PacketForwardingRules struct {
	pdrs []pdr
	fars []far
	qers []qer
}

// PFCPSession implements one PFCP session.
type PFCPSession struct {
	localSEID        uint64
	remoteSEID       uint64
	notificationFlag notifyFlag
	metrics          *metrics.Session
	PacketForwardingRules
}

func (p PacketForwardingRules) String() string {
	return fmt.Sprintf("PDRs=%v, FARs=%v, QERs=%v", p.pdrs, p.fars, p.qers)
}

// NewPFCPSession allocates an session with ID.
func (pConn *PFCPConn) NewPFCPSession(rseid uint64) uint64 {
	for i := 0; i < pConn.maxRetries; i++ {
		lseid := pConn.rng.Uint64()
		// Check if it already exists
		if _, ok := pConn.store.GetSession(lseid); ok {
			continue
		}

		s := PFCPSession{
			localSEID:  lseid,
			remoteSEID: rseid,
			PacketForwardingRules: PacketForwardingRules{
				pdrs: make([]pdr, 0, MaxItems),
				fars: make([]far, 0, MaxItems),
				qers: make([]qer, 0, MaxItems),
			},
		}
		s.metrics = metrics.NewSession(pConn.nodeID.remote)

		pConn.store.UpdateSession(s)

		// Metrics update
		pConn.SaveSessions(s.metrics)

		return lseid
	}

	return 0
}

// RemoveSession removes session using lseid.
func (pConn *PFCPConn) RemoveSession(lseid uint64) {
	s, ok := pConn.store.GetSession(lseid)
	if !ok {
		return
	}

	// Metrics update
	s.metrics.Delete()
	pConn.SaveSessions(s.metrics)

	pConn.store.RemoveSession(lseid)
}
