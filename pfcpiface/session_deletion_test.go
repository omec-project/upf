// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"slices"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// The session is the only record of which rules a deletion has to remove. Once it is
// gone, a rule the datapath still holds is unreachable: no later message names it,
// because the control plane has been told the session no longer exists. So the deletion
// handler must not remove it on a write that did not finish -- and that is a decision for
// the deletion handler, not for SendMsgToUPF, because a modification that removes rules
// wants the opposite.

// unfinishingDP answers every write the way bess answers one that ran out of time: the
// cause SendMsgToUPF gives is accepted, and the write is reported as not finished.
type unfinishingDP struct {
	fakeDP

	writes []upfMsgType
}

func (d *unfinishingDP) SendMsgToUPF(method upfMsgType, all, newRules PacketForwardingRules) uint8 {
	cause, _ := d.SendMsgToUPFWithCompletion(method, all, newRules)
	return cause
}

func (d *unfinishingDP) SendMsgToUPFWithCompletion(
	method upfMsgType, _, _ PacketForwardingRules,
) (uint8, bool) {
	d.writes = append(d.writes, method)
	return ie.CauseRequestAccepted, false
}

// deletionConn is an association holding one established session, on a datapath that
// finishes the establishment and then stops finishing anything.
func deletionConn(t *testing.T) (*PFCPConn, *unfinishingDP, uint64) {
	t.Helper()

	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	dp := &unfinishingDP{}
	pConn.upf.datapath = dp

	return pConn, dp, localSEID
}

func deletionCause(t *testing.T, rsp message.Message) uint8 {
	t.Helper()

	sdres, ok := rsp.(*message.SessionDeletionResponse)
	if !ok {
		t.Fatalf("expected a Session Deletion Response, got %T", rsp)
	}

	cause, err := sdres.Cause.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause of the deletion response: %v", err)
	}

	return cause
}

// assertDeletionSEID checks the header SEID of a deletion response: the control plane's
// SEID for the session wherever the session was found, so that a refusal can be matched to
// it, and zero only where it was not.
func assertDeletionSEID(t *testing.T, rsp message.Message, want uint64) {
	t.Helper()

	if got := rsp.SEID(); got != want {
		t.Fatalf("the deletion response carries header SEID %#x, expected %#x", got, want)
	}
}

func TestADeletionTheDatapathDidNotFinishKeepsTheSession(t *testing.T) {
	pConn, _, localSEID := deletionConn(t)

	rsp, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err == nil {
		t.Fatal("a deletion the datapath did not finish was answered as done")
	}

	if cause := deletionCause(t, rsp); cause != ie.CauseRequestRejected {
		t.Fatalf("the deletion was answered with cause %d, expected %d",
			cause, ie.CauseRequestRejected)
	}

	if _, held := pConn.store.GetSession(localSEID); !held {
		t.Fatal("the session was removed although the datapath did not finish removing its " +
			"rules; nothing names whatever it had not removed, and no later message can")
	}

	assertDeletionSEID(t, rsp, testRemoteSEID)
}

// TestADeletionTheDatapathFinishedStillRemovesTheSession is the guard on the other side:
// the handler must not have become unable to delete.
func TestADeletionTheDatapathFinishedStillRemovesTheSession(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	rsp, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err != nil {
		t.Fatalf("a deletion the datapath finished was refused: %v", err)
	}

	if cause := deletionCause(t, rsp); cause != ie.CauseRequestAccepted {
		t.Fatalf("the deletion was answered with cause %d, expected %d",
			cause, ie.CauseRequestAccepted)
	}

	if _, held := pConn.store.GetSession(localSEID); held {
		t.Fatal("the session is still stored after a deletion the datapath finished")
	}
}

// TestAModificationWhoseRemovalDidNotFinishIsNotRolledBack pins why this is decided at the
// deletion handler rather than in SendMsgToUPF. A modification's session is live and is
// kept whatever the answer, so refusing its unfinished removal gains nothing -- and would
// send it into a rollback whose restore the late removal could then undo, leaving a live
// session without rules the store still describes. So that path goes on seeing accepted.
func TestAModificationWhoseRemovalDidNotFinishIsNotRolledBack(t *testing.T) {
	pConn, dp, localSEID := deletionConn(t)

	rsp, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
			ie.NewRemoveFAR(ie.NewFARID(downlinkFARID))))
	if err != nil {
		t.Fatalf("a modification whose removal did not finish was refused: %v", err)
	}

	smres, ok := rsp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", rsp)
	}

	assertAccepted(t, smres.Cause, "modification")

	// The handler always writes the modification and then the removal, even with nothing
	// to modify. A rollback would follow them with a removal and a restore of its own.
	want := []upfMsgType{upfMsgTypeMod, upfMsgTypeDel}
	if !slices.Equal(dp.writes, want) {
		t.Fatalf("the modification wrote %v to the datapath, expected %v; a rollback ran "+
			"after a removal that had not finished", dp.writes, want)
	}
}

// refusingDP finishes every write and refuses it, under either method: embedding does not
// dispatch, so overriding only the completion one would leave SendMsgToUPF answering
// accepted, and a handler that calls that one would be tested against a datapath that
// never refused anything.
type refusingDP struct{ fakeDP }

func (d *refusingDP) SendMsgToUPF(_ upfMsgType, _, _ PacketForwardingRules) uint8 {
	return ie.CauseRequestRejected
}

func (d *refusingDP) SendMsgToUPFWithCompletion(
	_ upfMsgType, _, _ PacketForwardingRules,
) (uint8, bool) {
	return ie.CauseRequestRejected, true
}

// TestADeletionTheDatapathRefusedKeepsTheSession covers the other half of the condition:
// a write that finished and was refused. Consulting whether it finished must not have
// displaced consulting what it answered.
func TestADeletionTheDatapathRefusedKeepsTheSession(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	pConn.upf.datapath = &refusingDP{}

	rsp, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err == nil {
		t.Fatal("a deletion the datapath refused was answered as done")
	}

	if cause := deletionCause(t, rsp); cause != ie.CauseRequestRejected {
		t.Fatalf("the deletion was answered with cause %d, expected %d",
			cause, ie.CauseRequestRejected)
	}

	if _, held := pConn.store.GetSession(localSEID); !held {
		t.Fatal("the session was removed although the datapath refused to remove its rules")
	}

	assertDeletionSEID(t, rsp, testRemoteSEID)
}

// TestADeletionOfAnUnknownSessionCarriesSEIDZero is the guard on the SEID: with no session
// there is no control-plane SEID to give, and 29.244 clause 7.2.2.4.2 says zero.
func TestADeletionOfAnUnknownSessionCarriesSEIDZero(t *testing.T) {
	pConn, _, localSEID := deletionConn(t)

	rsp, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID+1, 3, 0))
	if err == nil {
		t.Fatal("a deletion of a session this UPF does not hold was answered as done")
	}

	assertDeletionSEID(t, rsp, 0)
}
