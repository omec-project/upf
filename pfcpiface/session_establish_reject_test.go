// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// An establishment that is refused while still being parsed never reaches the datapath,
// so the rollback that follows a refused batch never runs for it. It has nonetheless
// allocated: reading a CHOOSE PDR takes a UE address out of the pool before the rest of
// the message is known to be well formed, and NewPFCPSession has already counted the
// session. Neither is ever presented again -- the session is not stored, and the
// control plane is given no F-SEID it could delete.

// countingMetrics mirrors what the Prometheus session gauge does with a saved session:
// a session with no duration is new, one with a duration has been deleted.
type countingMetrics struct {
	live int
}

func (c *countingMetrics) SaveMessages(*metrics.Message) {}

func (c *countingMetrics) SaveSessions(s *metrics.Session) {
	if s.Duration == 0 {
		c.live++
		return
	}

	c.live--
}

func (c *countingMetrics) Stop() error { return nil }

// choosePDR is a well-formed downlink PDR asking the UPF to allocate the UE address.
func choosePDR() *ie.IE {
	return ie.NewCreatePDR(
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(0),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			chooseUEAddress(),
		),
		ie.NewFARID(downlinkFARID),
	)
}

// rejectEstablishment sends an establishment that must be answered with wantCause. The
// cause is asserted rather than returned because it is what says the refusal came from
// the parse loop: a request refused before NewPFCPSession -- a NodeID that does not match
// the association, say -- has allocated nothing and counted nothing, so it would satisfy
// every assertion in this file while exercising none of it.
func rejectEstablishment(t *testing.T, pConn *PFCPConn, wantCause uint8, rules ...*ie.IE) {
	t.Helper()

	ies := []*ie.IE{
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(0xC0FFEE, net.ParseIP("127.0.0.1"), nil),
	}
	ies = append(ies, rules...)

	rsp, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0, ies...))
	if err == nil {
		t.Fatal("the establishment was accepted, so this test proves nothing")
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	if seres.Cause == nil {
		t.Fatal("the rejection carries no Cause IE")
	}

	cause, err := seres.Cause.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause of the rejection: %v", err)
	}

	if cause != wantCause {
		t.Fatalf("the establishment was refused with cause %d, expected %d; "+
			"a refusal from elsewhere in the handler does not reach what this test covers",
			cause, wantCause)
	}
}

// TestAPDRThatFailedToParseReleasesTheAddressItAllocated is the one the session's own
// rules cannot reach: the PDR allocates while its PDI is read and then fails on the FAR
// ID that follows, so it is never appended to the session at all.
func TestAPDRThatFailedToParseReleasesTheAddressItAllocated(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)

	// A CHOOSE PDR with no FAR ID. parsePDR reads the PDI -- allocating -- before it
	// reads the FAR ID it has no way to supply.
	rejectEstablishment(t, pConn, ie.CauseRequestRejected, ie.NewCreatePDR(
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(0),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			chooseUEAddress(),
		),
	))

	assertPoolIsWhole(t, pool, "an establishment whose PDR failed to parse")
}

// TestAMalformedFARReleasesTheAddressAnEarlierPDRAllocated is the same leak one loop
// later: the PDR is good and in the session, and the message is refused after it.
func TestAMalformedFARReleasesTheAddressAnEarlierPDRAllocated(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)

	// A CreateFAR with no FAR ID: parseFAR fails on the first field it reads.
	rejectEstablishment(t, pConn, ie.CauseRequestRejected, choosePDR(), ie.NewCreateFAR(
		ie.NewApplyAction(ActionForward),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
	))

	assertPoolIsWhole(t, pool, "an establishment whose FAR failed to parse")
}

// TestARejectedEstablishmentIsNotCountedAsALiveSession covers the other thing
// NewPFCPSession hands out before the session is known to exist. The gauge it feeds is
// the UPF's count of live sessions, so a session abandoned mid-parse inflates it for
// the life of the process.
func TestARejectedEstablishmentIsNotCountedAsALiveSession(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	counter := &countingMetrics{}
	pConn := establishingConn(t, pool)
	pConn.InstrumentPFCP = counter

	rejectEstablishment(t, pConn, ie.CauseRequestRejected, choosePDR(), ie.NewCreateFAR(
		ie.NewApplyAction(ActionForward),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
	))

	if counter.live != 0 {
		t.Fatalf("%d session(s) still counted as live after a rejected establishment; "+
			"the abandoned session was never removed", counter.live)
	}
}
