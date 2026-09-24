// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"net"
	"slices"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// A Session Modification Request is parsed rule by rule into the session the store
// already holds, and any rule that fails to parse refuses the whole message. The
// control plane is then told the session is unchanged -- so the store has to say the
// same thing.

const (
	establishedPrecedence = 0
	modifiedPrecedence    = 42
	secondPDRID           = downlinkPDRID + 1
	unknownFARID          = 99
	testRemoteSEID        = 0xC0FFEE
	applicationQERID      = 1
	sessionQERID          = 2
	lowerMBR              = 100
	higherMBR             = 200
	highestMBR            = 300
	firstUEAddress        = "10.250.0.1"
	secondUEAddress       = "10.250.0.2"
	thirdUEAddress        = "10.250.0.3"
)

// corePDR is a downlink PDR whose UE address the control plane assigned, so nothing in
// this file depends on the UPF's address pool.
func corePDR(pdrID uint16, ueAddress string, extra ...*ie.IE) *ie.IE {
	ies := []*ie.IE{
		ie.NewPDRID(pdrID),
		ie.NewPrecedence(establishedPrecedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, ueAddress, "", 0, 0),
		),
		ie.NewFARID(downlinkFARID),
	}

	return ie.NewCreatePDR(append(ies, extra...)...)
}

// establishStaticSession establishes a session with the given rules and returns the
// local SEID the store is keyed by.
func establishStaticSession(t *testing.T, pConn *PFCPConn, rules ...*ie.IE) uint64 {
	t.Helper()

	ies := []*ie.IE{
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(testRemoteSEID, net.ParseIP("127.0.0.1"), nil),
	}
	ies = append(ies, rules...)

	rsp, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0, ies...))
	if err != nil {
		t.Fatalf("establishment refused: %v", err)
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	assertAccepted(t, seres.Cause, "establishment")

	fseid, err := seres.UPFSEID.FSEID()
	if err != nil {
		t.Fatalf("could not read the UP F-SEID: %v", err)
	}

	return fseid.SEID
}

// refuseModification sends a modification that must be refused at the parse loop the
// caller names.
//
// The error alone does not say where the refusal came from: the session lookup's wraps
// the same errNotFound a Remove of an absent rule does. What separates them is the
// response -- the lookup answers "Session context not found" under SEID 0, every return
// after it CauseRequestRejected under the session's SEID. Both are asserted, because a
// test whose fixture stopped reaching the loops would otherwise find the store unchanged
// for the wrong reason and pass.
func refuseModification(t *testing.T, pConn *PFCPConn, localSEID uint64, at error, ies ...*ie.IE) {
	t.Helper()

	rsp, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, localSEID, 2, 0, ies...))
	if err == nil {
		t.Fatal("the modification was accepted, so this test proves nothing")
	}

	if !errors.Is(err, at) {
		t.Fatalf("the modification was refused with %q, expected a refusal at %q", err, at)
	}

	smres, ok := rsp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", rsp)
	}

	if smres.SEID() != testRemoteSEID {
		t.Fatalf("the rejection carries SEID %#x, expected %#x; the handler refused the "+
			"message before it found the session, so it reached none of the parse loops",
			smres.SEID(), uint64(testRemoteSEID))
	}

	if smres.Cause == nil {
		t.Fatal("the rejection carries no Cause IE")
	}

	cause, err := smres.Cause.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause of the rejection: %v", err)
	}

	if cause != ie.CauseRequestRejected {
		t.Fatalf("the modification was refused with cause %d, expected %d",
			cause, ie.CauseRequestRejected)
	}
}

// TestAModificationOfAnUnknownSessionIsAnsweredContextNotFound: 29.244 clause 7.2.2.4.2
// answers a message for a session the node has no context for with "Session context not
// found" under header SEID 0.
func TestAModificationOfAnUnknownSessionIsAnsweredContextNotFound(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	rsp, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, localSEID+1, 2, 0))
	if !errors.Is(err, errNotFound) {
		t.Fatalf("a modification of a session this UPF does not hold was refused with %v, "+
			"expected a not-found error", err)
	}

	smres, ok := rsp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", rsp)
	}

	if smres.SEID() != 0 {
		t.Fatalf("the response carries SEID %#x, expected 0", smres.SEID())
	}

	if smres.Cause == nil {
		t.Fatal("the response carries no Cause IE")
	}

	cause, err := smres.Cause.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause of the response: %v", err)
	}

	if cause != ie.CauseSessionContextNotFound {
		t.Fatalf("the modification was answered with cause %d, expected %d",
			cause, ie.CauseSessionContextNotFound)
	}
}

func storedSession(t *testing.T, pConn *PFCPConn, localSEID uint64) PFCPSession {
	t.Helper()

	session, ok := pConn.store.GetSession(localSEID)
	if !ok {
		t.Fatal("the session is not in the store")
	}

	return session
}

// TestARefusedModificationDoesNotApplyTheUpdatesItParsed covers the update loops. The
// PDR and the FAR parse and are applied; the QER after them carries no QER ID, so the
// message is refused before any of it reaches the datapath.
func TestARefusedModificationDoesNotApplyTheUpdatesItParsed(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	refuseModification(t, pConn, localSEID, ie.ErrIENotFound,
		ie.NewUpdatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(modifiedPrecedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, firstUEAddress, "", 0, 0),
			),
			ie.NewFARID(downlinkFARID),
		),
		ie.NewUpdateFAR(ie.NewFARID(downlinkFARID), ie.NewApplyAction(ActionDrop)),
		// No QER ID: parseQER fails on the first field it reads.
		ie.NewUpdateQER(ie.NewQFI(1)),
	)

	session := storedSession(t, pConn, localSEID)

	if len(session.pdrs) != 1 || len(session.fars) != 1 {
		t.Fatalf("expected the session's 1 PDR and 1 FAR, got %d and %d",
			len(session.pdrs), len(session.fars))
	}

	if session.pdrs[0].precedence != establishedPrecedence {
		t.Fatalf("the stored PDR has precedence %d, expected %d; the refused "+
			"modification was applied to the session the UPF says it did not change",
			session.pdrs[0].precedence, establishedPrecedence)
	}

	if session.fars[0].applyAction != ActionForward {
		t.Fatalf("the stored FAR applies action %d, expected %d; the refused "+
			"modification was applied to the session the UPF says it did not change",
			session.fars[0].applyAction, ActionForward)
	}
}

// TestARefusedModificationDoesNotRemoveTheRulesItParsed covers the removal loops, which
// splice the session's slice rather than assigning into it. Two PDRs are needed for the
// splice to move anything: removing the only one leaves the array untouched.
func TestARefusedModificationDoesNotRemoveTheRulesItParsed(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress),
		corePDR(secondPDRID, secondUEAddress), downlinkFAR())

	refuseModification(t, pConn, localSEID, errNotFound,
		ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)),
		// A FAR the session does not hold, which the handler refuses.
		ie.NewRemoveFAR(ie.NewFARID(unknownFARID)),
	)

	session := storedSession(t, pConn, localSEID)

	if len(session.pdrs) != 2 {
		t.Fatalf("expected the session's 2 PDRs, got %d", len(session.pdrs))
	}

	for i, want := range []uint32{downlinkPDRID, secondPDRID} {
		if session.pdrs[i].pdrID != want {
			t.Fatalf("stored PDR %d has ID %d, expected %d; the refused modification "+
				"removed a rule from the session the UPF says it did not change",
				i, session.pdrs[i].pdrID, want)
		}
	}
}

// TestARefusedModificationDoesNotReorderAStoredQERList covers the one field of a rule
// that is a slice of its own. MarkSessionQer moves the session QER to the end of every
// PDR's QER list, in place, and a modification that changes which QER that is reorders
// the list of a PDR the message never mentioned.
func TestARefusedModificationDoesNotReorderAStoredQERList(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress,
			ie.NewQERID(applicationQERID), ie.NewQERID(sessionQERID)),
		downlinkFAR(),
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		ie.NewCreateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)),
	)

	// The session QER is the one with the larger MBR, so raising the other one's makes
	// it the session QER instead -- and MarkSessionQer then has a list to reorder.
	refuseModification(t, pConn, localSEID, errNotFound,
		ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(highestMBR, highestMBR)),
		// A FAR the session does not hold, which the handler refuses.
		ie.NewRemoveFAR(ie.NewFARID(unknownFARID)),
	)

	session := storedSession(t, pConn, localSEID)

	if len(session.pdrs) != 1 {
		t.Fatalf("expected the session's 1 PDR, got %d", len(session.pdrs))
	}

	want := []uint32{applicationQERID, sessionQERID}
	if !slices.Equal(session.pdrs[0].qerIDList, want) {
		t.Fatalf("the stored PDR's QER list is %v, expected %v; the refused "+
			"modification reordered a rule the UPF says it did not change",
			session.pdrs[0].qerIDList, want)
	}

	if session.qers[0].ulMbr != lowerMBR {
		t.Fatalf("the stored QER has an uplink MBR of %d, expected %d; the refused "+
			"modification was applied to the session the UPF says it did not change",
			session.qers[0].ulMbr, lowerMBR)
	}
}
