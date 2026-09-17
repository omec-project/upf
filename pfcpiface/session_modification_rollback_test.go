// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
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
