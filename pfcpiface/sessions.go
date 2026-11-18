// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"fmt"
	"slices"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

type PacketForwardingRules struct {
	pdrs []pdr
	fars []far
	qers []qer
	urrs []urr
}

// PFCPSession implements one PFCP session.
type PFCPSession struct {
	localSEID  uint64
	remoteSEID uint64
	metrics    *metrics.Session
	PacketForwardingRules
}

func (p PacketForwardingRules) String() string {
	return fmt.Sprintf("PDRs=%v, FARs=%v, QERs=%v, URRs=%v", p.pdrs, p.fars, p.qers, p.urrs)
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

// holdsQEREntry reports whether a QER of q's ID in this set occupies q's datapath entry.
func (p PacketForwardingRules) holdsQEREntry(q qer) bool {
	return slices.ContainsFunc(p.qers, func(v qer) bool {
		return v.qerID == q.qerID && q.occupiesSameEntryAs(v)
	})
}

// holdsPDREntry reports whether a PDR of p's ID in this set occupies p's datapath entry.
func (p PacketForwardingRules) holdsPDREntry(r pdr) bool {
	return slices.ContainsFunc(p.pdrs, func(v pdr) bool {
		return v.pdrID == r.pdrID && r.occupiesSameEntryAs(v)
	})
}

// remarked returns what the session QER marking changed among the rules before held, as
// this set holds them now: the PDRs whose first QER, the one BESS looks their traffic up
// by, is not the one they were written with, and the QERs whose level is not the one they
// were written at. A QER that changed level keeps the entry it had, since a PDR that
// names only the session QER still reaches the datapath through its application entry;
// if a quota had closed that entry and no QER this set holds takes it over, it is
// returned too, open. This set's rules below the length of before's are before's, at
// their positions, with a message's updates applied in place.
func (p PacketForwardingRules) remarked(before PacketForwardingRules) ([]pdr, []qer) {
	var pdrs []pdr

	for i, was := range before.pdrs {
		if firstQERID(p.pdrs[i]) != firstQERID(was) {
			pdrs = append(pdrs, p.pdrs[i])
		}
	}

	var qers []qer

	for i, was := range before.qers {
		if p.qers[i].qosLevel == was.qosLevel {
			continue
		}

		qers = append(qers, p.qers[i])

		if was.quotaClosed && !slices.ContainsFunc(p.qers, was.sharesEntryWith) {
			was.quotaClosed = false
			qers = append(qers, was)
		}
	}

	return pdrs, qers
}

// firstQERID is the QER ID addPDR writes into a PDR's entry: the first of its list, or 0.
func firstQERID(p pdr) uint32 {
	if len(p.qerIDList) == 0 {
		return 0
	}

	return p.qerIDList[0]
}

// heldVersions returns the rules a message created or updated, as the session holds them,
// with the created ones first and their count. held is the session's rules after the
// message's creates and updates: the rules it had, at their positions and with updates
// applied in place, followed by what the message created. So everything past kept is
// created -- in its final version, where an Update replaced it -- and a rule before that
// is the message's only if an Update named it. An Update replaces the first rule of its ID
// it finds, so only that one is: a session can hold two rules of one ID, and the other is
// untouched.
func heldVersions[R any](held []R, kept int, updated []R, id func(R) uint32) ([]R, int) {
	rules := append([]R(nil), held[kept:]...)
	created := len(rules)
	seen := map[uint32]bool{}

	for _, r := range held[:kept] {
		if seen[id(r)] {
			continue
		}

		seen[id(r)] = true

		for _, u := range updated {
			if id(u) == id(r) {
				rules = append(rules, r)
				break
			}
		}
	}

	return rules, created
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
		urrs: append(make([]urr, 0, cap(p.urrs)), p.urrs...),
	}

	// qerIDList and urrIDList are the fields of a rule that are slices of their own, and
	// MarkSessionQer rewrites the first in place.
	for i := range c.pdrs {
		c.pdrs[i].qerIDList = append(make([]uint32, 0, cap(p.pdrs[i].qerIDList)), p.pdrs[i].qerIDList...)
		c.pdrs[i].urrIDList = append([]uint32(nil), p.pdrs[i].urrIDList...)
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
