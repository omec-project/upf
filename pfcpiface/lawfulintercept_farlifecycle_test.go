// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"

	"github.com/omec-project/util/li/types"
)

// withoutFAR is the session with one FAR removed, as a session modification carrying a
// RemoveFAR leaves it.
func withoutFAR(s PFCPSession, farID uint32) PFCPSession {
	out := s
	out.fars = nil
	for i := range s.fars {
		if s.fars[i].farID != farID {
			out.fars = append(out.fars, s.fars[i])
		}
	}

	return out
}

// TestARemovedFARLeavesNoRecordBehind is the reclamation half of the leak.
//
// A session modification that removes a FAR takes it out of the session's own list, and every
// path that reclaims this record walks that list: sessionProgrammed over the FARs that remain,
// sessionForgotten over the same list at teardown. So the entry for the FAR that went is
// unreachable from both, and a re-derivation — which does rebuild the record from live
// sessions — runs only when something asks for one. A removal that leaves nothing duplicating
// asks for none.
//
// The element goes on intercepting correctly the entire time, which is what makes this
// invisible: the only symptom is a record that grows with every FAR any subscriber has ever
// had removed, until the process dies and takes every warrant it holds with it.
func TestARemovedFARLeavesNoRecordBehind(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(100, testUEIPv4)
	f.putSession(t, sess)

	// Tasked by F-TEID rather than UE address, so exactly one of the session's two FARs
	// duplicates. That is what makes the removal below ask for no re-derivation — the FARs
	// that remain are unchanged and none of them is duplicating — and a removal that asks
	// for no pass is a removal nothing reclaims. Tasking the whole session would have hidden
	// the leak behind the pass the *other* FAR's liveness requests.
	f.activate(t, "W1", types.TargetIdentifier{Type: types.TargetFTEID, Value: "4196"})

	if value, held := f.recorded(100, 1); !held || !value {
		t.Fatalf("FAR 1 recorded as (%v, held=%v) before the removal; the interception this "+
			"test removes was never running", value, held)
	}
	if f.duplicates(t, 100, 2) {
		t.Fatal("the F-TEID criterion duplicated the downlink FAR too, so this test is not " +
			"exercising the case it describes")
	}

	before := f.pushCount()

	// The modification, in the handler's own order: the session in the store loses the FAR,
	// then the element is told, then the remaining FARs are re-recorded.
	remaining := withoutFAR(sess, 1)
	if err := f.store.PutSession(remaining); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	f.e.farsRemoved(100, []far{{farID: 1, fseID: 100}})
	// A pure removal pushes no created or updated rules, so the handler records none — which
	// is the point: the FARs that remain keep the record they already had.
	f.e.sessionProgrammed(&remaining, nil, true)
	f.settle(t)

	if f.pushCount() != before {
		t.Fatalf("the removal caused %d push(es); this test asserts what happens when nothing "+
			"asks for a re-derivation, and a pass would reclaim the entry itself",
			f.pushCount()-before)
	}

	if _, held := f.recorded(100, 1); held {
		t.Error("the element still records what the datapath was told about a FAR that no " +
			"longer exists; nothing that walks the session will ever reclaim it")
	}
	// The FAR that stayed must be untouched — a reclamation that took the session's live
	// entries with it would leave transact with no record of a FAR it still has to answer for.
	if _, held := f.recorded(100, 2); !held {
		t.Error("the removal of one FAR discarded the record of another the session still has")
	}
}

// TestAPassDoesNotReAddAFARTheSMFRemoved is the sharper half, and it is a datapath-integrity
// failure rather than a bookkeeping one.
//
// A re-derivation plans from the session list it read at the start of the pass. If a
// modification removes one of those FARs before the push, the pass pushes it anyway — and the
// datapath's modify path programs what it is given, so it *re-creates* a forwarding rule the
// SMF has deleted. The interception plane would be putting back a rule the control plane
// removed, which is the one thing a re-derivation's own contract says it must never do.
func TestAPassDoesNotReAddAFARTheSMFRemoved(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(100, testUEIPv4)
	f.putSession(t, sess)
	w := f.windowed(t)

	// Tasking installed without asking for a pass, so the pass this test drives is the one
	// that would first program duplication for FAR 1.
	task := ccTask("W1", ueAddr(testUEIPv4))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	// A pass begins and reads the sessions, FAR 1 among them.
	w.hold <- struct{}{}
	gen := f.e.request()
	<-w.read

	before := f.pushCount()

	// The modification arrives inside the interval: FAR 1 is gone from the session and from
	// the datapath, and the element is told.
	remaining := withoutFAR(sess, 1)
	if err := f.store.PutSession(remaining); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	f.e.farsRemoved(100, []far{{farID: 1, fseID: 100}})

	// The pass now acts on a session list that still contains FAR 1.
	w.release <- struct{}{}
	f.e.await(gen)
	f.settle(t)

	f.mu.Lock()
	pushes := append([]PacketForwardingRules(nil), f.pushed[before:]...)
	f.mu.Unlock()

	for _, p := range pushes {
		for i := range p.fars {
			if p.fars[i].farID == 1 {
				t.Fatalf("the pass pushed FAR 1 after the SMF removed it, which re-creates a "+
					"forwarding rule the control plane deleted (%d push(es) after the removal)",
					len(pushes))
			}
		}
	}

	if _, held := f.recorded(100, 1); held {
		t.Error("the pass wrote its own conclusion about the removed FAR back into the record, " +
			"which is the deletion being undone by the pass that was told about it")
	}
}

// TestDuplicationPushedBeforeAFailedRemovalIsStillWithdrawable covers a refused modification
// that leaves duplication running, which is over-collection.
//
// The modification handler pushes its created and updated rules to the datapath first and
// processes the removals second. A failure in that removal stage returns a rejection — before
// PutSession, and therefore before sessionProgrammed, which is the only thing that records
// what was pushed. So the datapath is duplicating and the element holds no record of it: a
// later re-derivation computes the tasking's answer, finds no entry, reads that as "not
// duplicating", and where the tasking now says it should not be duplicating either, concludes
// there is nothing to do.
//
// The copies keep being made for a subject no warrant covers, and nothing in the element can
// turn them off. The assertion is therefore on the datapath: an element that merely recorded
// correctly and left the traffic being copied would pass a test pinned to the record.
func TestDuplicationPushedBeforeAFailedRemovalIsStillWithdrawable(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(100, testUEIPv4)
	f.putSession(t, sess)

	// Tasking installed without a pass, so it is the modification's own push that starts the
	// interception — which is what leaves it unrecorded.
	task := ccTask("W1", ueAddr(testUEIPv4))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	// The handler's first half: rules re-derived over the modification and the changed subset
	// pushed to the datapath.
	modified := sess
	modified.fars = append([]far(nil), sess.fars...)
	updated := PacketForwardingRules{fars: append([]far(nil), modified.fars...)}
	f.e.applyTasking(&modified, &updated)
	f.record(updated)

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the modification did not start the interception the tasking required")
	}

	// The removal stage fails. The session is never stored and sessionProgrammed never runs;
	// the handler records the push as attempted, because its rollback may have undone it.
	f.e.farsAttempted(100, updated.fars)

	if _, held := f.recorded(100, 1); !held {
		t.Fatal("FAR 1 holds no record after a push the handler could not store; the " +
			"element does not know the datapath may be duplicating")
	}

	// The warrant is withdrawn. This is the only event that will ever revisit the session.
	if !f.tasks.Deactivate(types.XID("W1")) {
		t.Fatal("Deactivate found no task")
	}
	f.e.retaskAndWait()
	f.settle(t)

	if f.duplicates(t, 100, 1) {
		t.Error("the datapath is still duplicating a withdrawn warrant's traffic: the push " +
			"that enabled it was never recorded, so the withdrawal's own pass found nothing " +
			"to turn off")
	}
}
