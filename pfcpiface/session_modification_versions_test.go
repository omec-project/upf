// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
)

// An accepted modification is written to the datapath and then stored, and a later
// deletion removes what the store describes. So every entry the write makes has to be one
// the stored session names, at the key the session names it by -- anything else outlives
// the session in the datapath. For a QER the key includes its table, which its level
// picks.

// acceptedWrite returns the rules an accepted modification wrote.
func acceptedWrite(t *testing.T, dp *recordingDP) PacketForwardingRules {
	t.Helper()

	if len(dp.calls) == 0 || dp.calls[0].method != upfMsgTypeMod {
		t.Fatalf("the modification did not start with a write of its rules: %v", dp.calls)
	}

	return dp.calls[0].updated
}

// twoQerConn is a session whose PDR carries an application QER and, with the larger MBR,
// the session QER -- so the establishment's marking puts them in different tables.
func twoQerConn(t *testing.T) (*PFCPConn, *recordingDP, uint64) {
	t.Helper()

	dp := &recordingDP{}
	pConn := establishingConn(t, nil)
	pConn.upf.datapath = dp

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress,
			ie.NewQERID(applicationQERID), ie.NewQERID(sessionQERID)),
		downlinkFAR(),
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		ie.NewCreateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)))

	session := storedSession(t, pConn, localSEID)
	if q, _ := session.findQER(sessionQERID); q.qosLevel != SessionQos {
		t.Fatalf("the establishment stored QER %d at level %d, expected the session level; "+
			"the fixture is not the one this file is about", sessionQERID, q.qosLevel)
	}

	dp.calls = nil

	return pConn, dp, localSEID
}

// assertWrittenAsStored checks that the write carries QER id exactly once, with the MBR
// given, at the level the stored session records it at.
func assertWrittenAsStored(t *testing.T, pConn *PFCPConn, dp *recordingDP, localSEID uint64,
	id uint32, mbr uint64,
) {
	t.Helper()

	stored, ok := storedSession(t, pConn, localSEID).findQER(id)
	if !ok {
		t.Fatalf("the session does not hold QER %d", id)
	}

	var written []qer

	for _, q := range acceptedWrite(t, dp).qers {
		if q.qerID == id {
			written = append(written, q)
		}
	}

	if len(written) != 1 {
		t.Fatalf("the write carries QER %d %d times, expected once: %v; the versions are "+
			"programmed concurrently, and the session holds one", id, len(written), written)
	}

	if written[0].ulMbr != mbr {
		t.Errorf("QER %d was written with MBR %d, expected %d", id, written[0].ulMbr, mbr)
	}

	if written[0].qosLevel != stored.qosLevel {
		t.Errorf("QER %d was written to the %s lookup while the session records it in the %s "+
			"one; a deletion removes it from the one recorded, and leaves the other",
			id, qosLevelName[written[0].qosLevel], qosLevelName[stored.qosLevel])
	}
}

// TestAnUpdateOfTheSessionQerAloneIsWrittenWhereTheSessionHoldsIt is a session AMBR
// change: one Update QER, for the session QER. Marked on the message's own QERs, a single
// QER is never a session one, so it was written to the application lookup -- the session
// lookup kept the old rate, and the new entry was one no deletion would name.
func TestAnUpdateOfTheSessionQerAloneIsWrittenWhereTheSessionHoldsIt(t *testing.T) {
	pConn, dp, localSEID := twoQerConn(t)

	modifySession(t, pConn, localSEID,
		ie.NewUpdateQER(ie.NewQERID(sessionQERID), ie.NewMBR(highestMBR, highestMBR)))

	assertWrittenAsStored(t, pConn, dp, localSEID, sessionQERID, highestMBR)
}

// TestAnUpdateOfBothQersIsStillWrittenWhereTheSessionHoldsThem is the guard on the common
// case, where the message's QERs are the session's and both markings agree.
func TestAnUpdateOfBothQersIsStillWrittenWhereTheSessionHoldsThem(t *testing.T) {
	pConn, dp, localSEID := twoQerConn(t)

	modifySession(t, pConn, localSEID,
		ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(higherMBR, higherMBR)),
		ie.NewUpdateQER(ie.NewQERID(sessionQERID), ie.NewMBR(highestMBR, highestMBR)))

	assertWrittenAsStored(t, pConn, dp, localSEID, applicationQERID, higherMBR)
	assertWrittenAsStored(t, pConn, dp, localSEID, sessionQERID, highestMBR)
}

// TestACreateThenUpdateOfOneQerIsWrittenOnce: UpdateQER finds its ID in the session as the
// message has built it so far, so the Update replaces the QER the Create just added. Both
// versions used to be written, concurrently, and marked on each other: the larger MBR in
// the session lookup, the Create's in the application one where the session records the
// Update's.
func TestACreateThenUpdateOfOneQerIsWrittenOnce(t *testing.T) {
	dp := &recordingDP{}
	pConn := establishingConn(t, nil)
	pConn.upf.datapath = dp

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewQERID(applicationQERID)),
		downlinkFAR())

	dp.calls = nil

	modifySession(t, pConn, localSEID,
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(highestMBR, highestMBR)))

	assertWrittenAsStored(t, pConn, dp, localSEID, applicationQERID, highestMBR)
}

// TestACreateThenUpdateOfOnePdrIsWrittenOnce is the same for a PDR, where the Update can
// give the rule another key: the Create's entry would be one the session no longer names.
func TestACreateThenUpdateOfOnePdrIsWrittenOnce(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID,
		corePDR(secondPDRID, secondUEAddress),
		ie.NewUpdatePDR(
			ie.NewPDRID(secondPDRID),
			ie.NewPrecedence(establishedPrecedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, thirdUEAddress, "", 0, 0),
			),
			ie.NewFARID(downlinkFARID),
		))

	var written []pdr

	for _, p := range acceptedWrite(t, dp).pdrs {
		if p.pdrID == secondPDRID {
			written = append(written, p)
		}
	}

	if len(written) != 1 || written[0].appFilter.dstIP != ip2int(net.ParseIP(thirdUEAddress)) {
		t.Fatalf("the write carries PDR %d as %v, expected once, at %s; the session holds "+
			"only the Update's version", secondPDRID, written, thirdUEAddress)
	}
}

// TestACreateThenUpdateOfOneFarIsWrittenOnce is the same for a FAR. Its key cannot move, so
// what is at stake is which version the datapath ends up holding: the two are written
// concurrently to one entry.
func TestACreateThenUpdateOfOneFarIsWrittenOnce(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID,
		ie.NewCreateFAR(
			ie.NewFARID(secondFARID),
			ie.NewApplyAction(ActionForward),
			ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
		),
		ie.NewUpdateFAR(ie.NewFARID(secondFARID), ie.NewApplyAction(ActionDrop)))

	stored := storedSession(t, pConn, localSEID)

	var want far

	for _, f := range stored.fars {
		if f.farID == secondFARID {
			want = f
		}
	}

	var written []far

	for _, f := range acceptedWrite(t, dp).fars {
		if f.farID == secondFARID {
			written = append(written, f)
		}
	}

	if len(written) != 1 || written[0].applyAction != want.applyAction {
		t.Fatalf("the write carries FAR %d as %v, expected once, as the session holds it: %v",
			secondFARID, written, want)
	}
}

// writtenPDRAddresses returns the UE addresses the write carries PDR id at.
func writtenPDRAddresses(t *testing.T, dp *recordingDP, id uint32) map[string]bool {
	t.Helper()

	at := map[string]bool{}

	for _, p := range acceptedWrite(t, dp).pdrs {
		if p.pdrID == id {
			at[int2ip(p.appFilter.dstIP).String()] = true
		}
	}

	return at
}

// TestTwoCreatesOfOneRuleAreBothWritten: CreatePDR appends, so the session holds both
// rules, and a write of only the later one would leave the store naming a rule the
// datapath was never given.
func TestTwoCreatesOfOneRuleAreBothWritten(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID,
		corePDR(secondPDRID, secondUEAddress),
		corePDR(secondPDRID, thirdUEAddress))

	at := writtenPDRAddresses(t, dp, secondPDRID)
	if len(at) != 2 || !at[secondUEAddress] || !at[thirdUEAddress] {
		t.Fatalf("the write carries PDR %d at %v, expected both %s and %s, as the session "+
			"holds it", secondPDRID, at, secondUEAddress, thirdUEAddress)
	}
}

// TestACreateOfAHeldRuleThenAnUpdateWritesBoth: the Create appends a second rule of an ID
// the session already holds, and UpdatePDR replaces the first rule of that ID it finds --
// the one the session had. So the session holds the Create's rule and the Update's, and
// the write has to carry both.
func TestACreateOfAHeldRuleThenAnUpdateWritesBoth(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID,
		corePDR(downlinkPDRID, secondUEAddress),
		updateTheDownlinkPDR(thirdUEAddress, establishedPrecedence))

	at := writtenPDRAddresses(t, dp, downlinkPDRID)
	if len(at) != 2 || !at[secondUEAddress] || !at[thirdUEAddress] {
		t.Fatalf("the write carries PDR %d at %v, expected both %s, the Create's, and %s, "+
			"the Update's of the rule the session had", downlinkPDRID, at,
			secondUEAddress, thirdUEAddress)
	}
}

// TestAModificationWritesOnlyTheRulesItTouched: a rule the message did not name is already
// programmed as the session holds it. Written again it gains nothing, and it races with
// any Create in the same batch that shares its datapath key.
func TestAModificationWritesOnlyTheRulesItTouched(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID, dropTheDownlinkFAR())

	written := acceptedWrite(t, dp)
	if len(written.pdrs) != 0 || len(written.qers) != 0 || len(written.fars) != 1 {
		t.Fatalf("a modification updating one FAR wrote %d PDRs, %d FARs and %d QERs, "+
			"expected only that FAR", len(written.pdrs), len(written.fars), len(written.qers))
	}
}

// duplicatedPDRConn is a session holding PDR downlinkPDRID twice -- its own, at the first
// address, and one an accepted Create of the same ID added at the second. Nothing rejects
// that Create; CreatePDR appends.
func duplicatedPDRConn(t *testing.T) (*PFCPConn, *recordingDP, uint64) {
	t.Helper()

	pConn, dp, localSEID := rollbackConn(t)

	modifySession(t, pConn, localSEID, corePDR(downlinkPDRID, secondUEAddress))

	dp.calls = nil

	return pConn, dp, localSEID
}

// TestAnUpdateOfADuplicatedIDWritesOnlyTheRuleItReplaced: UpdatePDR replaces the first rule
// of its ID it finds, so the session's second one is untouched and has no business in
// the write.
func TestAnUpdateOfADuplicatedIDWritesOnlyTheRuleItReplaced(t *testing.T) {
	pConn, dp, localSEID := duplicatedPDRConn(t)

	modifySession(t, pConn, localSEID,
		updateTheDownlinkPDR(thirdUEAddress, establishedPrecedence))

	at := writtenPDRAddresses(t, dp, downlinkPDRID)
	if len(at) != 1 || !at[thirdUEAddress] {
		t.Fatalf("the write carries PDR %d at %v, expected only %s, the rule the Update "+
			"replaced", downlinkPDRID, at, thirdUEAddress)
	}
}

// TestARollbackLeavesTheDuplicateAnUpdateDidNotReplace is where writing the untouched
// duplicate would do harm: the rollback compares each updated rule with the first rule of
// its ID before the message, so the duplicate, at another key, reads as moved -- and is
// removed until the restore puts it back a batch later, a flow dropped for no reason.
func TestARollbackLeavesTheDuplicateAnUpdateDidNotReplace(t *testing.T) {
	pConn, dp, localSEID := duplicatedPDRConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		updateTheDownlinkPDR(thirdUEAddress, establishedPrecedence))

	for _, p := range rollbackDeletion(t, dp).pdrs {
		if p.appFilter.dstIP == ip2int(net.ParseIP(secondUEAddress)) {
			t.Fatalf("the rollback removed PDR %d at %s, the rule of that ID the Update did "+
				"not touch", p.pdrID, secondUEAddress)
		}
	}
}
