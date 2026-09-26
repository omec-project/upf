// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"slices"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// A refused establishment is rolled back with a removal of everything the refused batch
// may have programmed, and then forgotten: it is never stored, and the control plane is
// given no F-SEID it could delete. That is only safe once the removal has finished. One
// that ran out of time may have left rules in the datapath, and a forgotten session
// leaves nothing that names them -- while its UE address goes back to the pool for
// another UE, whose traffic a surviving PDR would then match.

// addRefusingDP refuses every add. A removal is refused when refuseDel is set; otherwise
// it is accepted, and finished or not as delFinishes says -- an unfinished one accepted,
// because that is how bess answers a batch that ran out of time.
type addRefusingDP struct {
	fakeDP

	delFinishes bool
	refuseDel   bool
	writes      []upfMsgType
}

func (d *addRefusingDP) SendMsgToUPF(method upfMsgType, all, newRules PacketForwardingRules) uint8 {
	cause, _ := d.SendMsgToUPFWithCompletion(method, all, newRules)
	return cause
}

func (d *addRefusingDP) SendMsgToUPFWithCompletion(
	method upfMsgType, _, _ PacketForwardingRules,
) (uint8, bool) {
	d.writes = append(d.writes, method)

	switch {
	case method == upfMsgTypeAdd:
		return ie.CauseRequestRejected, true
	case d.refuseDel:
		return ie.CauseRequestRejected, true
	default:
		return ie.CauseRequestAccepted, d.delFinishes
	}
}

// establishRollbackConn is an association on a two-address pool whose datapath refuses the
// establishment's batch, with the gauge counted.
func establishRollbackConn(t *testing.T, dp *addRefusingDP) (*PFCPConn, *IPPool, *countingMetrics) {
	t.Helper()

	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	pConn.upf.datapath = dp

	counter := &countingMetrics{}
	pConn.InstrumentPFCP = counter

	rsp, err := pConn.handleSessionEstablishmentRequest(message.NewSessionEstablishmentRequest(
		0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(testRemoteSEID, net.ParseIP("127.0.0.1"), nil),
		choosePDR(), downlinkFAR()))
	if err == nil {
		t.Fatal("an establishment whose batch the datapath refused was accepted")
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	if cause, err := seres.Cause.Cause(); err != nil || cause != ie.CauseRequestRejected {
		t.Fatalf("the establishment was answered with cause %d (%v), expected %d",
			cause, err, ie.CauseRequestRejected)
	}

	if want := []upfMsgType{upfMsgTypeAdd, upfMsgTypeDel}; !slices.Equal(dp.writes, want) {
		t.Fatalf("the establishment wrote %v to the datapath, expected %v; "+
			"the refusal did not come from the batch", dp.writes, want)
	}

	return pConn, pool, counter
}

func TestARejectedEstablishmentWhoseRollbackDidNotFinishKeepsTheSession(t *testing.T) {
	dp := &addRefusingDP{delFinishes: false}
	pConn, pool, counter := establishRollbackConn(t, dp)

	sessions := pConn.store.GetAllSessions()
	if len(sessions) != 1 {
		t.Fatalf("%d session(s) stored after a rollback that did not finish, expected 1; "+
			"nothing names whatever rules the rollback had not removed", len(sessions))
	}

	if counter.live != 1 {
		t.Fatalf("%d session(s) counted as live, expected the kept one", counter.live)
	}

	// The kept session still holds its address, so only one of the two is free.
	if _, err := pool.LookupOrAllocIP(0xE001); err != nil {
		t.Fatalf("the pool has no free address at all: %v", err)
	}

	if _, err := pool.LookupOrAllocIP(0xE002); err == nil {
		t.Fatal("the address the kept session was given went back to the pool, " +
			"while a PDR the rollback may not have removed still matches it")
	}

	pool.Release(0xE001)

	// The association teardown is what removes it, rules and address together.
	dp.writes = nil

	pConn.executeShutdown()

	if !slices.Equal(dp.writes, []upfMsgType{upfMsgTypeDel}) {
		t.Fatalf("the teardown wrote %v to the datapath, expected one removal of the "+
			"kept session", dp.writes)
	}

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("%d session(s) stored after the teardown", n)
	}

	assertPoolIsWhole(t, pool, "a teardown of a kept rejected session")
}

// TestARejectedEstablishmentWhoseRollbackFinishedIsForgotten is the guard on the other
// side: a rollback that finished leaves nothing to name, so nothing is kept.
func TestARejectedEstablishmentWhoseRollbackFinishedIsForgotten(t *testing.T) {
	pConn, pool, counter := establishRollbackConn(t, &addRefusingDP{delFinishes: true})

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("%d session(s) stored after a rejected establishment whose rollback "+
			"finished", n)
	}

	if counter.live != 0 {
		t.Fatalf("%d session(s) still counted as live", counter.live)
	}

	assertPoolIsWhole(t, pool, "a rejected establishment whose rollback finished")
}

// TestARejectedEstablishmentWhoseRollbackWasRefusedIsForgotten pins why only completion
// keeps the session. A refused removal of a rule the datapath holds cannot happen today:
// what refuses one is a rule that could not be translated or marshalled, and was
// therefore never programmed either. Keeping the session for it would hold a real
// address for nothing.
func TestARejectedEstablishmentWhoseRollbackWasRefusedIsForgotten(t *testing.T) {
	pConn, pool, counter := establishRollbackConn(t, &addRefusingDP{refuseDel: true})

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("%d session(s) stored after a rejected establishment whose rollback "+
			"was refused", n)
	}

	if counter.live != 0 {
		t.Fatalf("%d session(s) still counted as live", counter.live)
	}

	assertPoolIsWhole(t, pool, "a rejected establishment whose rollback was refused")
}
