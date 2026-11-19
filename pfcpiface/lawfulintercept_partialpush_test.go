// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
)

// divergeThenModify builds the one state in which the record's authority matters: a FAR the
// datapath is duplicating that this element's tasking says should not be duplicated.
//
// Reached the way it is reached in production — a warrant is withdrawn and the datapath refuses
// the write that turns duplication off. transact restores the record to what the datapath
// actually holds, which is the whole reason it keeps prior values rather than writing its
// intent. From here the record and the intent disagree, and every mechanism that reads one in
// place of the other becomes visible.
//
// Then the SMF modifies the session, restating FAR 1 and not FAR 2. Only FAR 1 reaches the
// datapath. What the element records about FAR 2 is the subject of both tests below.
func divergeThenModify(t *testing.T, f *enablerFixture) {
	t.Helper()

	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	if !f.duplicates(t, 100, 1) || !f.duplicates(t, 100, 2) {
		t.Fatal("the tasked session is not duplicated, so nothing below is exercised")
	}

	// The datapath stops accepting, and the warrant is withdrawn. The write that would turn
	// duplication off is refused, so the datapath goes on duplicating both FARs.
	f.mu.Lock()
	f.cause = ie.CauseRequestRejected
	f.mu.Unlock()
	f.deactivate(t, "W1")
	f.settle(t)

	if !f.duplicates(t, 100, 1) || !f.duplicates(t, 100, 2) {
		t.Fatal("the fake datapath applied a write it refused; this fixture is not modelling a " +
			"refusal and the divergence this test needs does not exist")
	}
	for _, farID := range []uint32{1, 2} {
		if value, held := f.recorded(100, farID); !held || !value {
			t.Fatalf("after a refused withdrawal the record for FAR %d says held=%v duplicating=%v; "+
				"it must still describe the datapath, which is duplicating", farID, held, value)
		}
	}

	// The modification. There is no tasking now, so applyTasking marks nothing: the session's
	// own FARs all read "not duplicating", which is this element's *intent* and not the
	// datapath's state.
	sess, ok := f.store.GetSession(100)
	if !ok {
		t.Fatal("the session went missing")
	}
	restated := far{farID: 1, fseID: 100, applyAction: ActionForward}
	for i := range sess.fars {
		if sess.fars[i].farID == 1 {
			sess.fars[i] = restated
		} else {
			sess.fars[i].liDuplicate = false
		}
	}
	pushed := PacketForwardingRules{fars: []far{restated}}
	f.e.applyTasking(&sess, &pushed)
	f.record(pushed)
	f.commitModification(t, sess, pushed.fars)
}

// TestAPartialModificationLeavesTheOtherRulesRecordAlone: the record says what the datapath was
// told, not what this element wants.
//
// A Session Modification restates only the rules the SMF is changing, and its copies carry no
// notion of duplication. The handler pushes those and only those — so a record rewritten from
// the session's whole FAR list claims the datapath was told this element's intended value for
// every rule the message did not contain.
//
// **The error runs in the one direction that cannot be recovered from.** A FAR still duplicating
// whose record says otherwise is invisible to the next pass: the pass compares the record
// against the tasking, and where the tasking agrees, concludes there is nothing to do. The
// copies go on being made under no warrant, and the element's own account says duplication is
// off — over-collection recorded as compliance.
func TestAPartialModificationLeavesTheOtherRulesRecordAlone(t *testing.T) {
	f := newEnablerFixture(t)
	divergeThenModify(t, f)

	if value, held := f.recorded(100, 2); !held || !value {
		t.Errorf("a FAR the modification never pushed is recorded as held=%v duplicating=%v, and "+
			"the datapath is duplicating it. The record is this element's only account of what "+
			"the datapath holds, and it now describes what the element wanted instead",
			held, value)
	}
}

// TestARuleLeftUnpushedIsStillProgrammedOffWhenTheDatapathRecovers is the consequence.
//
// The record earns its keep at exactly one moment: a pass turning duplication off skips any FAR
// whose recorded value already equals what the tasking implies. A FAR recorded as "not
// duplicating" while the datapath duplicates it is therefore never pushed off — the warrant is
// gone, the copies continue, and nothing in the element will ask about that FAR again.
func TestARuleLeftUnpushedIsStillProgrammedOffWhenTheDatapathRecovers(t *testing.T) {
	f := newEnablerFixture(t)
	divergeThenModify(t, f)

	// The datapath starts accepting writes again, and something asks for a re-derivation — a
	// tasking change, or the periodic one. There is no tasking, so nothing should be duplicated.
	f.mu.Lock()
	f.cause = 0
	f.mu.Unlock()
	f.e.retaskAndWait()
	f.settle(t)

	if f.duplicates(t, 100, 1) {
		t.Error("the restated FAR is still duplicating with no warrant in place")
	}
	if f.duplicates(t, 100, 2) {
		t.Error("a FAR the modification did not push is still duplicating with no warrant in " +
			"place: its record said duplication was already off, so the pass that turns it off " +
			"had nothing to do. Content is being copied under no warrant at all, and nothing " +
			"left in this element can turn it off")
	}
}

// TestADivergentRecordDoesNotMakeOverCollectionPermanent is the assertion that bounds the harm
// when the record is wrong in the one direction it must never be wrong in.
//
// The record is the element's only memory of what the datapath holds, and the re-derivation
// skips any FAR whose recorded value already equals what the tasking implies. So a record that
// says "not duplicating" while the datapath *is* would, without everDuplicated, be unrecoverable:
// the element computes "nothing should duplicate", sees its record agreeing, and does nothing —
// while a subscriber's traffic goes on being copied under no authority. Invisible from both ends,
// because the element's own account says duplication is off and the copies are dropped as
// unattributable rather than delivered.
//
// **This test does not reproduce a cause.** More than one path can leave the record diverged, so
// it corrupts the record directly, which is the state whatever the cause, and asserts the
// element recovers on the next pass.
func TestADivergentRecordDoesNotMakeOverCollectionPermanent(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the tasked session is not duplicated, so there is no divergence to create")
	}

	// The divergence: the datapath is duplicating and the element's record says it is not.
	// Written directly rather than provoked, because the provoking path is unknown — and a test
	// that waited for it would assert nothing.
	f.e.mu.Lock()
	f.e.programmed[farRef{seid: 100, farID: 1}] = programmedFAR{duplicating: false, written: f.e.writes}
	f.e.mu.Unlock()

	if value, held := f.recorded(100, 1); !held || value {
		t.Fatalf("the record was not corrupted as intended (held=%v value=%v)", held, value)
	}

	// The warrant is withdrawn. Every copy from here on is over-collection, and the record the
	// element would normally trust says there is nothing to turn off.
	f.deactivate(t, "W1")
	f.settle(t)

	if f.duplicates(t, 100, 1) {
		t.Error("the datapath is still duplicating after the warrant was withdrawn, because the " +
			"element's record claimed it was already off. A subscriber's traffic is being copied " +
			"under no authority, the element's own account says otherwise, and no later " +
			"re-derivation will look at this rule again")
	}
}

// TestAFARNeverTurnedOnIsNotPushedRepeatedly keeps the remedy from costing what it saves.
//
// Refusing to trust the record in the "off" direction is only affordable because it applies to
// FARs this element has *ever* turned on, which is a small and shrinking set. Applied to every
// FAR it would mean a full datapath rewrite on every pass, for sessions no warrant has ever
// touched.
func TestAFARNeverTurnedOnIsNotPushedRepeatedly(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	// A second session no criterion will ever select.
	f.putSession(t, unmarkedSession(200, "10.250.0.10"))
	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	before := f.pushCount()
	// Several passes, with no tasking change that concerns the untouched session.
	for range 3 {
		f.e.retaskAndWait()
	}
	f.settle(t)

	// The untouched session's FARs were never duplicating, so nothing about them should reach
	// the datapath however many passes run.
	if f.duplicates(t, 200, 1) || f.duplicates(t, 200, 2) {
		t.Error("a session no warrant covers is being duplicated")
	}
	if after := f.pushCount(); after > before+3 {
		t.Errorf("%d pushes for %d passes over a session nothing selects; the remedy must apply "+
			"to FARs this element has turned on, not to every FAR it can see",
			after-before, 3)
	}
}

// TestADivergenceIsNoticedWithoutATaskingChange completes the previous test.
//
// That one withdraws the warrant, which is a tasking change and therefore asks for a
// re-derivation by itself. This one changes no tasking at all: it corrupts the record while no
// warrant covers the session, and then does nothing but a session event.
//
// It matters because the remedy lives in the re-derivation, and in the diverged state both the
// record and the session's own flag read false — so the test that decides whether to *ask* for a
// re-derivation would see nothing notable and never ask. Distrusting the record when deciding what to
// do, while trusting it when deciding whether to look, leaves the datapath duplicating until some
// unrelated event happens along.
func TestADivergenceIsNoticedWithoutATaskingChange(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the tasked session is not duplicated")
	}

	// Withdraw first, so no tasking change follows the corruption.
	f.deactivate(t, "W1")
	f.settle(t)
	if f.duplicates(t, 100, 1) {
		t.Fatal("duplication did not stop on withdrawal, so this test cannot isolate what it means to")
	}

	// Now put the datapath back into the diverged state by hand: duplicating, with the record
	// and the session's own flag both saying otherwise.
	sess, ok := f.store.GetSession(100)
	if !ok {
		t.Fatal("the session went missing")
	}
	teed := sess.fars[0]
	teed.liDuplicate = true
	f.record(PacketForwardingRules{fars: []far{teed}})
	if !f.duplicates(t, 100, teed.farID) {
		t.Fatal("the fake datapath did not take the tee, so there is no divergence")
	}

	// A session event, and nothing else. No tasking changes here.
	f.e.sessionProgrammed(&sess, nil, true)
	f.settle(t)

	if f.duplicates(t, 100, teed.farID) {
		t.Error("the datapath is still duplicating after a session event, with no warrant in " +
			"place. Nothing asked for a re-derivation, because the record and the session both " +
			"claimed duplication was already off — so the remedy never ran and a subscriber's " +
			"traffic is copied until some unrelated event happens along")
	}
}
