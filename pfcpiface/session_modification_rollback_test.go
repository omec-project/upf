// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
)

// A Session Modification Request is written to the datapath in the middle of the
// handler, and the handler can still refuse the message after that -- a Remove naming a
// rule the session does not hold, or a deletion batch the datapath rejects. The control
// plane is then told the session is unchanged, so the datapath has to be put back to
// what it held before the message.

const secondFARID = downlinkFARID + 1

type dpCall struct {
	method  upfMsgType
	rules   PacketForwardingRules
	updated PacketForwardingRules
}

// recordingDP records every write and can refuse one of them. The rollback's own writes
// are accepted, so what a test reads back is the rollback the handler asked for.
type recordingDP struct {
	fakeDP

	calls           []dpCall
	refuseNextWrite bool
}

func (d *recordingDP) SendMsgToUPF(method upfMsgType, rules, updated PacketForwardingRules) uint8 {
	d.calls = append(d.calls, dpCall{method: method, rules: rules, updated: updated})

	if d.refuseNextWrite {
		d.refuseNextWrite = false
		return ie.CauseRequestRejected
	}

	return ie.CauseRequestAccepted
}

// rollbackConn is a session of one PDR and one forwarding FAR, with the establishment's
// own write already forgotten so a test reads only what its modification caused.
func rollbackConn(t *testing.T) (*PFCPConn, *recordingDP, uint64) {
	t.Helper()

	dp := &recordingDP{}
	pConn := establishingConn(t, nil)
	pConn.upf.datapath = dp

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	dp.calls = nil

	return pConn, dp, localSEID
}

// dropTheDownlinkFAR is an update a test can see in the datapath: the session was
// established with a FAR that forwards.
func dropTheDownlinkFAR() *ie.IE {
	return ie.NewUpdateFAR(ie.NewFARID(downlinkFARID), ie.NewApplyAction(ActionDrop))
}

// assertRolledBack checks the two writes a rollback is: the rules the session had are
// written again, and the rules this message created are removed.
func assertRolledBack(t *testing.T, dp *recordingDP, wantCreated PacketForwardingRules) {
	t.Helper()

	if len(dp.calls) != 3 {
		t.Fatalf("the modification made %d datapath calls, expected 3 -- its own write, "+
			"the removal and the restore: %v", len(dp.calls), dp.calls)
	}

	remove := dp.calls[1]
	if remove.method != upfMsgTypeDel {
		t.Fatalf("the first rollback call is a %v, expected a %v", remove.method, upfMsgTypeDel)
	}

	if len(remove.rules.pdrs) != len(wantCreated.pdrs) ||
		len(remove.rules.fars) != len(wantCreated.fars) ||
		len(remove.rules.qers) != len(wantCreated.qers) {
		t.Fatalf("the rollback removed %v, expected only what the message created: %v",
			remove.rules, wantCreated)
	}

	for i, p := range wantCreated.pdrs {
		if remove.rules.pdrs[i].pdrID != p.pdrID {
			t.Fatalf("the rollback removed PDR %d, expected the created PDR %d",
				remove.rules.pdrs[i].pdrID, p.pdrID)
		}
	}

	for i, f := range wantCreated.fars {
		if remove.rules.fars[i].farID != f.farID {
			t.Fatalf("the rollback removed FAR %d, expected the created FAR %d",
				remove.rules.fars[i].farID, f.farID)
		}
	}

	restore := dp.calls[2]
	if restore.method != upfMsgTypeMod {
		t.Fatalf("the second rollback call is a %v, expected a %v", restore.method, upfMsgTypeMod)
	}

	// The session was established with one PDR and one FAR that forwards. Anything else
	// means the restore wrote the modified rules rather than the ones it replaced.
	if len(restore.updated.pdrs) != 1 || restore.updated.pdrs[0].pdrID != downlinkPDRID {
		t.Fatalf("the restore wrote PDRs %v, expected only the session's PDR %d",
			restore.updated.pdrs, downlinkPDRID)
	}

	if len(restore.updated.fars) != 1 || restore.updated.fars[0].applyAction != ActionForward {
		t.Fatalf("the restore wrote FARs %v, expected only the session's forwarding FAR",
			restore.updated.fars)
	}
}

// TestAModificationTheDatapathRefusedIsRolledBack covers the write refusing itself. The
// batch has programmed an unknown subset of its rules by then, so the rules the session
// had have to be written again.
func TestAModificationTheDatapathRefusedIsRolledBack(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath, dropTheDownlinkFAR())

	assertRolledBack(t, dp, PacketForwardingRules{})
}

// TestAModificationRefusedAfterItsWriteIsRolledBack covers the refusals the datapath
// never sees: the write was accepted, and a Remove after it names a FAR the session does
// not hold. Without the rollback the datapath keeps the dropping FAR while the control
// plane is told the modification did not happen.
func TestAModificationRefusedAfterItsWriteIsRolledBack(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	refuseModification(t, pConn, localSEID, errNotFound,
		dropTheDownlinkFAR(), ie.NewRemoveFAR(ie.NewFARID(unknownFARID)))

	assertRolledBack(t, dp, PacketForwardingRules{})
}

// TestARolledBackModificationRemovesOnlyWhatItCreated is the other half. A rule the
// message created did not exist before it, so restoring cannot undo it -- it has to go.
// A rule the message updated must not, or the rollback would leave the session with less
// than it started with.
func TestARolledBackModificationRemovesOnlyWhatItCreated(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		corePDR(secondPDRID, secondUEAddress),
		ie.NewCreateFAR(
			ie.NewFARID(secondFARID),
			ie.NewApplyAction(ActionForward),
			ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
		),
		dropTheDownlinkFAR(),
	)

	assertRolledBack(t, dp, PacketForwardingRules{
		pdrs: []pdr{{pdrID: secondPDRID}},
		fars: []far{{farID: secondFARID}},
	})
}

// TestAModificationRefusedBeforeItsWriteLeavesTheDatapathAlone is the guard on the other
// side: a message refused while it is still being parsed has changed nothing, and a
// rollback of it would write rules the datapath already holds.
func TestAModificationRefusedBeforeItsWriteLeavesTheDatapathAlone(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)

	// No QER ID: parseQER fails before the handler reaches the datapath.
	refuseModification(t, pConn, localSEID, ie.ErrIENotFound, ie.NewUpdateQER(ie.NewQFI(1)))

	if len(dp.calls) != 0 {
		t.Fatalf("a modification refused before its write made %d datapath calls, expected none: %v",
			len(dp.calls), dp.calls)
	}
}

// lastWriteOn reports what the datapath was last asked to do with a FAR ID, which is
// what decides whether the rule is there once the rollback has finished.
func lastWriteOn(dp *recordingDP, farID uint32) (upfMsgType, bool) {
	var (
		last  upfMsgType
		found bool
	)

	for _, c := range dp.calls {
		rules := c.rules
		if c.method == upfMsgTypeMod {
			rules = c.updated
		}

		for _, f := range rules.fars {
			if f.farID == farID {
				last, found = c.method, true
			}
		}
	}

	return last, found
}

// TestARollbackDoesNotRemoveARuleACreateNamedTwice is why the removal runs before the
// restore. Nothing rejects a Create naming a rule the session already holds -- CreateFAR
// appends without looking at the IDs already there -- and the datapath keys a FAR by its
// ID and F-SEID, so the created rule and the session's own are one entry. Removing the
// created rule after restoring would take the restored rule with it.
func TestARollbackDoesNotRemoveARuleACreateNamedTwice(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		ie.NewCreateFAR(
			ie.NewFARID(downlinkFARID),
			ie.NewApplyAction(ActionDrop),
		),
	)

	method, found := lastWriteOn(dp, downlinkFARID)
	if !found {
		t.Fatal("the datapath was never asked to do anything with the session's FAR")
	}

	if method != upfMsgTypeDel {
		return
	}

	t.Fatal("the rollback's last act on the session's FAR was to remove it: the created " +
		"rule shared the session's own datapath key, so removing it after the restore " +
		"took the restored rule away, and the session now describes a rule the datapath " +
		"does not hold")
}

// A rule the message updated is written at whatever key its new content gives it. When
// that key is not the one the session's own version occupies, the write has left an entry
// behind that the restore does not touch: the restore re-adds the old key, it does not
// remove the new one. Only a rule whose key actually moved may be removed -- removing one
// whose key did not would take out the very entry the restore is about to put back, and
// blackhole a flow the message never changed.

// rollbackDeletion returns the rules the rollback asked the datapath to remove.
func rollbackDeletion(t *testing.T, dp *recordingDP) PacketForwardingRules {
	t.Helper()

	if len(dp.calls) != 3 {
		t.Fatalf("the modification made %d datapath calls, expected 3: %v", len(dp.calls), dp.calls)
	}

	if dp.calls[1].method != upfMsgTypeDel {
		t.Fatalf("the first rollback call is a %v, expected a %v",
			dp.calls[1].method, upfMsgTypeDel)
	}

	return dp.calls[1].rules
}

// updateTheDownlinkPDR rebuilds the session's PDR with the UE address and precedence
// given. The UE address is what a core PDR matches on, so changing it moves the rule's
// datapath key; the precedence is written as the entry's priority and is not part of it.
func updateTheDownlinkPDR(ueAddress string, precedence uint32) *ie.IE {
	return ie.NewUpdatePDR(
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(precedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, ueAddress, "", 0, 0),
		),
		ie.NewFARID(downlinkFARID),
	)
}

// TestARollbackRemovesAnUpdatedRuleTheWriteMoved is the entry nothing else will ever
// remove: the session describes the old key, so its deletion names that one, and the new
// key sits in the datapath until the process ends.
func TestARollbackRemovesAnUpdatedRuleTheWriteMoved(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		updateTheDownlinkPDR(secondUEAddress, establishedPrecedence))

	removed := rollbackDeletion(t, dp)

	if len(removed.pdrs) != 1 {
		t.Fatalf("the rollback removed %d PDRs, expected the one the write moved: %v",
			len(removed.pdrs), removed.pdrs)
	}

	// It has to be removed at the key the write gave it, not the one the session holds.
	if removed.pdrs[0].appFilter.dstIP != ip2int(net.ParseIP(secondUEAddress)) {
		t.Fatalf("the rollback removed the PDR at %v, expected the address the refused "+
			"write programmed it at (%s)",
			int2ip(removed.pdrs[0].appFilter.dstIP), secondUEAddress)
	}
}

// TestARollbackLeavesAnUpdatedRuleTheWriteDidNotMove is the other half, and the reason
// this cannot be done by removing every rule the write touched. The precedence is written
// as the entry's priority, which no delete keys on, so this update occupies exactly the
// entry the session's own version does.
func TestARollbackLeavesAnUpdatedRuleTheWriteDidNotMove(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		updateTheDownlinkPDR(firstUEAddress, modifiedPrecedence))

	removed := rollbackDeletion(t, dp)

	if len(removed.pdrs) != 0 {
		t.Fatalf("the rollback removed %v; that rule sits at the key the restore is "+
			"about to write, so removing it blackholes a flow the message never changed",
			removed.pdrs)
	}
}

// TestARollbackRemovesAQerTheWriteMovedBetweenTables is the same defect for a QER. The
// application and session QERs live in different datapath tables, so a QER the message
// moved between them was written to one while the session still describes the other.
func TestARollbackRemovesAQerTheWriteMovedBetweenTables(t *testing.T) {
	dp := &recordingDP{}
	pConn := establishingConn(t, nil)
	pConn.upf.datapath = dp

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress,
			ie.NewQERID(applicationQERID), ie.NewQERID(sessionQERID)),
		downlinkFAR(),
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		ie.NewCreateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)),
	)

	dp.calls = nil
	dp.refuseNextWrite = true

	// Raising the first QER's MBR above the second's swaps which of them is the session
	// QER, so both change tables. Both have to be in the message: MarkSessionQer only
	// marks the rules the message itself carries, and it does nothing at all below two
	// of them.
	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(highestMBR, highestMBR)),
		ie.NewUpdateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)))

	removed := rollbackDeletion(t, dp)

	if len(removed.qers) != 2 {
		t.Fatalf("the rollback removed %d QERs, expected the two the write moved "+
			"between tables: %v", len(removed.qers), removed.qers)
	}

	// Each has to be removed from the table the write put it in, not the one the session
	// still describes.
	levels := map[uint32]QosLevel{}
	for _, q := range removed.qers {
		levels[q.qerID] = q.qosLevel
	}

	if levels[applicationQERID] != SessionQos || levels[sessionQERID] != ApplicationQos {
		t.Fatalf("the rollback removed QER %d as %v and QER %d as %v; expected the levels "+
			"the refused write programmed them at, which are the other way round",
			applicationQERID, levels[applicationQERID], sessionQERID, levels[sessionQERID])
	}
}

// TestARollbackRemovesBothWhatItCreatedAndWhatItMoved covers the two sets together,
// which is the shape where they are drawn from the same slices.
func TestARollbackRemovesBothWhatItCreatedAndWhatItMoved(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		corePDR(secondPDRID, secondUEAddress),
		updateTheDownlinkPDR(thirdUEAddress, establishedPrecedence))

	removed := rollbackDeletion(t, dp)

	if len(removed.pdrs) != 2 {
		t.Fatalf("the rollback removed %d PDRs, expected the created one and the moved "+
			"one: %v", len(removed.pdrs), removed.pdrs)
	}

	at := map[uint32]uint32{}
	for _, p := range removed.pdrs {
		at[p.pdrID] = p.appFilter.dstIP
	}

	if at[secondPDRID] != ip2int(net.ParseIP(secondUEAddress)) {
		t.Errorf("the created PDR was removed at %v, expected %s",
			int2ip(at[secondPDRID]), secondUEAddress)
	}

	if at[downlinkPDRID] != ip2int(net.ParseIP(thirdUEAddress)) {
		t.Errorf("the moved PDR was removed at %v, expected the address the refused "+
			"write programmed it at (%s)", int2ip(at[downlinkPDRID]), thirdUEAddress)
	}
}

// TestARollbackLeavesAnUpdatedRuleWhoseWildcardIsSpeltDifferently is the other way a
// rule can look moved without moving. A PDR with no SDF Filter leaves its port ranges at
// their zero value, and one carrying a filter that matches any port sets them to
// 0-65535; both compile to the same ternary rule, so the datapath holds one entry, not
// two.
func TestARollbackLeavesAnUpdatedRuleWhoseWildcardIsSpeltDifferently(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		ie.NewUpdatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(establishedPrecedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, firstUEAddress, "", 0, 0),
				ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
			),
			ie.NewFARID(downlinkFARID),
		))

	removed := rollbackDeletion(t, dp)

	if len(removed.pdrs) != 0 {
		t.Fatalf("the rollback removed %v; both spellings of a wildcard port compile to "+
			"the same ternary rule, so that is the entry the restore is about to write",
			removed.pdrs)
	}
}

// TestARollbackRemovesARuleTheMessageCreatedAndThenMoved is the case where the created and
// updated sets name the same rule. UpdatePDR looks for its ID in the session as the message
// has built it so far, so an Update following a Create of the same ID replaces the rule
// the Create just added -- and the write programs both keys. The created rule has no
// version in before, which is exactly why it has to go at both of them.
func TestARollbackRemovesARuleTheMessageCreatedAndThenMoved(t *testing.T) {
	pConn, dp, localSEID := rollbackConn(t)
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		corePDR(secondPDRID, secondUEAddress),
		ie.NewUpdatePDR(
			ie.NewPDRID(secondPDRID),
			ie.NewPrecedence(establishedPrecedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, thirdUEAddress, "", 0, 0),
			),
			ie.NewFARID(downlinkFARID),
		),
	)

	removed := rollbackDeletion(t, dp)

	at := map[uint32]bool{}
	for _, p := range removed.pdrs {
		if p.pdrID == secondPDRID {
			at[p.appFilter.dstIP] = true
		}
	}

	for _, want := range []string{secondUEAddress, thirdUEAddress} {
		if !at[ip2int(net.ParseIP(want))] {
			t.Errorf("the rollback did not remove PDR %d at %s; the refused write programmed "+
				"it there, and no version of it existed before the message: %v",
				secondPDRID, want, removed.pdrs)
		}
	}
}

// TestARollbackRemovesAQerTheMessageCreatedAndThenMoved is the same case for a QER, where
// what moves is the table. MarkSessionQer marks the rules the message itself carries, and
// with a Create and an Update of one ID those are two entries: the larger MBR becomes the
// session QER and the other stays an application one. So the write puts the same QER in
// both lookups, and neither version existed before the message.
func TestARollbackRemovesAQerTheMessageCreatedAndThenMoved(t *testing.T) {
	dp := &recordingDP{}
	pConn := establishingConn(t, nil)
	pConn.upf.datapath = dp

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewQERID(applicationQERID)),
		downlinkFAR())

	dp.calls = nil
	dp.refuseNextWrite = true

	refuseModification(t, pConn, localSEID, ErrWriteToDatapath,
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(highestMBR, highestMBR)))

	removed := rollbackDeletion(t, dp)

	levels := map[QosLevel]bool{}
	for _, q := range removed.qers {
		if q.qerID == applicationQERID {
			levels[q.qosLevel] = true
		}
	}

	if !levels[ApplicationQos] || !levels[SessionQos] {
		t.Fatalf("the rollback removed QER %d from %v; the refused write put it in both the "+
			"application and the session lookup, and no version of it existed before the "+
			"message", applicationQERID, removed.qers)
	}
}
