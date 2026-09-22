// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"fmt"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

type PacketForwardingRules struct {
	pdrs []pdr
	fars []far
	qers []qer
}

// PFCPSession implements one PFCP session.
type PFCPSession struct {
	localSEID  uint64
	remoteSEID uint64
	metrics    *metrics.Session
	PacketForwardingRules
}

func (p PacketForwardingRules) String() string {
	return fmt.Sprintf("PDRs=%v, FARs=%v, QERs=%v", p.pdrs, p.fars, p.qers)
}

// findPDR returns the rule this set holds under the given ID.
func (p PacketForwardingRules) findPDR(id uint32) (pdr, bool) {
	for _, v := range p.pdrs {
		if v.pdrID == id {
			return v, true
		}
	}

	return pdr{}, false
}

// findQER returns the rule this set holds under the given ID.
func (p PacketForwardingRules) findQER(id uint32) (qer, bool) {
	for _, v := range p.qers {
		if v.qerID == id {
			return v, true
		}
	}

	return qer{}, false
}

// Clone returns rules that share no storage with these ones.
//
// A PFCPSession read from the store is a struct copy, but its three slices still point
// at the arrays the store holds: assigning into one of them -- which is what UpdatePDR,
// UpdateFAR and UpdateQER do -- is a write the store sees at once, and removing a rule
// shifts every rule after it through the same array. A handler that can still refuse
// the message it is parsing therefore has to work on a copy of its own, and publish it
// with PutSession only once the message has succeeded.
func (p PacketForwardingRules) Clone() PacketForwardingRules {
	c := PacketForwardingRules{
		pdrs: append(make([]pdr, 0, cap(p.pdrs)), p.pdrs...),
		fars: append(make([]far, 0, cap(p.fars)), p.fars...),
		qers: append(make([]qer, 0, cap(p.qers)), p.qers...),
	}

	// qerIDList is the one field of a rule that is a slice of its own, and
	// MarkSessionQer rewrites it in place.
	for i := range c.pdrs {
		c.pdrs[i].qerIDList = append(make([]uint32, 0, cap(p.pdrs[i].qerIDList)), p.pdrs[i].qerIDList...)
	}

	return c
}

// NewPFCPSession allocates an session with ID.
func (pConn *PFCPConn) NewPFCPSession(rseid uint64) (PFCPSession, bool) {
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

		// Metrics update
		pConn.SaveSessions(s.metrics)

		return s, true
	}

	return PFCPSession{}, false
}

// RemoveSession removes session using lseid.
func (pConn *PFCPConn) RemoveSession(session PFCPSession) {
	// Metrics update
	session.metrics.Delete()
	pConn.SaveSessions(session.metrics)

	if err := pConn.store.DeleteSession(session.localSEID); err != nil {
		logger.PfcpLog.Errorf("failed to delete PFCP session from store: %v", err)
	}
}
