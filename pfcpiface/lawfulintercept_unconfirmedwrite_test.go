// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"strings"
	"testing"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/util/li/x1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// A modification whose push the datapath refuses leaves this element unable to say whether
// the rules were applied. What it records about that write decides whether it ever tries
// again: transact skips a FAR whose record already agrees with the tasking unless the
// answer is "off", so a record claiming the rules are duplicating agrees with a live task
// and every later pass skips it — for the life of the session.
//
// The subject then has a warrant against them producing nothing, and an element answering
// that it is healthy, which an agency cannot distinguish from a subject who has gone quiet.
func TestARefusedModificationIsRewrittenByTheNextPass(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(100, testUEIPv4)
	f.putSession(t, sess)

	task := ccTask("W1", ueAddr(testUEIPv4))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	// The handler re-derives over the modification and pushes the changed subset. The
	// datapath refuses and — on this path — nothing lands, so f.record is not called.
	modified := sess
	modified.fars = append([]far(nil), sess.fars...)
	updated := PacketForwardingRules{fars: append([]far(nil), modified.fars...)}
	f.e.applyTasking(&modified, &updated)

	// This is the whole of what the handler does on the refused branch: it returns before
	// PutSession, so sessionProgrammed never runs.
	f.e.farsAttempted(100, updated.fars)

	// Not fatal: the record is the mechanism, the rewrite below is the harm. Failing here
	// and stopping would leave the harm unproven, and it is the harm that matters.
	if value, held := f.recorded(100, 1); !held || value {
		t.Errorf("FAR 1 recorded as (duplicating=%v, held=%v) after a write whose outcome is "+
			"unknown; a record that claims the write landed is what stops the element retrying",
			value, held)
	}

	// A later pass runs — a session event, a tasking change, anything.
	f.e.request()
	f.settle(t)

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the element never rewrote the rules, so the warrant produces nothing and " +
			"no later pass will look again")
	}
}

// The converse, without which the test above could be satisfied by rewriting everything on
// every pass — which would be unbounded datapath churn rather than a fix.
func TestAConfirmedWriteIsNotRewritten(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(101, "10.250.0.10")
	f.putSession(t, sess)

	task := ccTask("W1", ueAddr("10.250.0.10"))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	modified := sess
	modified.fars = append([]far(nil), sess.fars...)
	updated := PacketForwardingRules{fars: append([]far(nil), modified.fars...)}
	f.e.applyTasking(&modified, &updated)
	f.record(updated)
	f.e.recordFARs(101, updated.fars, true)

	if !f.duplicates(t, 101, 1) {
		t.Fatal("the fixture did not start the interception")
	}

	before := f.pushCount()
	f.e.request()
	f.settle(t)

	if after := f.pushCount(); after != before {
		t.Errorf("a confirmed write was rewritten: pushes %d -> %d, with the tasking unchanged",
			before, after)
	}
}

// A rule this element ever turned on is never trusted-to-be-off again, whatever the record
// says — that is what keeps a subscriber from being copied under no authority, and recording
// an unconfirmed write as "off" must not have weakened it.
func TestAnEverDuplicatedFARIsStillTurnedOffWhateverTheRecordSays(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(102, "10.250.0.11")
	f.putSession(t, sess)

	task := ccTask("W1", ueAddr("10.250.0.11"))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	modified := sess
	modified.fars = append([]far(nil), sess.fars...)
	updated := PacketForwardingRules{fars: append([]far(nil), modified.fars...)}
	f.e.applyTasking(&modified, &updated)
	f.record(updated)

	// The refused branch records it as attempted, which leaves the record saying "off" while
	// the datapath is duplicating — the state the whole bounding remedy exists for.
	f.e.farsAttempted(102, updated.fars)

	if !f.duplicates(t, 102, 1) {
		t.Fatal("the fixture did not start the interception")
	}

	f.deactivate(t, "W1")
	f.settle(t)

	if f.duplicates(t, 102, 1) {
		t.Fatal("the withdrawal did not reach the datapath: a record saying \"off\" was trusted " +
			"for a FAR this element had turned on, so the subscriber is still being copied")
	}
}

// Correcting the condition restores the interception at the next pass; it says nothing
// about the interval before that, in which an accepted warrant produced no product. An
// agency receiving nothing cannot tell that from a subject who is not communicating, and
// this element is the only party that knows the write was unconfirmed.
func TestARefusedModificationIsReported(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(103, "10.250.0.12")
	f.putSession(t, sess)

	task := ccTask("W1", ueAddr("10.250.0.12"))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}
	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	modified := sess
	modified.fars = append([]far(nil), sess.fars...)
	updated := PacketForwardingRules{fars: append([]far(nil), modified.fars...)}
	f.e.applyTasking(&modified, &updated)

	before := len(f.reported)
	f.e.farsAttempted(103, updated.fars)

	if len(f.reported) != before+1 {
		t.Fatalf("reports %d -> %d: an unconfirmed write for an accepted task was recorded "+
			"and not reported, so the interval where the warrant produced nothing has no "+
			"representation anywhere", before, len(f.reported))
	}
	if got := f.reported[len(f.reported)-1]; got != x1.NEIssueDuplicationRefused {
		t.Errorf("issue type = %q, want %q", got, x1.NEIssueDuplicationRefused)
	}
}

// The report belongs to the warrant, not to the datapath. A rule this element could not
// confirm, carrying no duplication, is an ordinary datapath failure and none of the ADMF's
// business — reporting it would spend the credibility of the element's most serious report.
func TestAnUnconfirmedWriteWithoutDuplicationIsNotReported(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(104, "10.250.0.13")
	f.putSession(t, sess)

	// No tasking activated, so nothing this session carries duplicates.
	plain := PacketForwardingRules{fars: append([]far(nil), sess.fars...)}

	before := len(f.reported)
	f.e.farsAttempted(104, plain.fars)

	if len(f.reported) != before {
		t.Errorf("reports %d -> %d: an unconfirmed write carrying no duplication was reported "+
			"to the ADMF, which has nothing to act on and is told so less credibly next time",
			before, len(f.reported))
	}
}

// The over-collection protection. Recording an
// unconfirmed write as "off" is only safe because everDuplicated still carries what was
// *intended* — that set is what makes transact distrust a record claiming "off" for a rule
// this element once turned on, and it is the reason the datapath cannot be left duplicating
// with nothing able to stop it.
func TestAnUnconfirmedWriteStillJoinsTheEverDuplicatedSet(t *testing.T) {
	f := newEnablerFixture(t)

	_, everBefore := f.recordSizes()
	f.e.farsAttempted(105, []far{{farID: 1, fseID: 105, liDuplicate: true}})
	_, everAfter := f.recordSizes()

	if everAfter != everBefore+1 {
		t.Fatalf("everDuplicated %d -> %d: a rule this element tried to turn on was not "+
			"recorded as ever-duplicated, so a later pass would trust a record saying \"off\" "+
			"and leave the datapath copying a subscriber under no authority",
			everBefore, everAfter)
	}
}

// Undetectability. The report goes to the provisioning function over X1; the general
// operator log must stay silent, or an operator can tell a tasked subscriber's session from
// any other by the fact that it produced a message at all.
func TestAnUnconfirmedDuplicationWriteIsSilentOnTheOperatorLog(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	orig := logger.PfcpLog
	logger.PfcpLog = zap.New(core).Sugar()
	t.Cleanup(func() { logger.PfcpLog = orig })

	f := newEnablerFixture(t)
	f.e.farsAttempted(106, []far{{farID: 1, fseID: 106, liDuplicate: true}})

	if n := logs.Len(); n != 0 {
		t.Errorf("an unconfirmed duplication write emitted %d operator-log entries, want 0 "+
			"(undetectability): %v", n, logs.All())
	}
}

// The interrogation half, asserted rather than reasoned about: a task interrogation answers
// duplicationNotProgrammed after an unconfirmed write instead of claiming the task is
// faultless. Reading taskFaults suggests it does; this test is what shows it.
func TestAnUnconfirmedWriteIsVisibleToAnInterrogation(t *testing.T) {
	f := newEnablerFixture(t)
	sess := unmarkedSession(107, "10.250.0.14")
	f.putSession(t, sess)

	// activate, not tasks.Activate: the fixture's helper also drives the re-derivation that
	// builds the enabler's tasking snapshot, and resolveTaskFaults answers nil without one.
	f.activate(t, "W1", ueAddr("10.250.0.14"))
	f.settle(t)

	// The refused branch: recorded as attempted, so the record no longer claims the rules
	// are duplicating.
	f.e.farsAttempted(107, []far{{farID: 1, fseID: 107, liDuplicate: true}})

	faults := f.e.taskFaults("W1")
	if len(faults) == 0 {
		t.Fatal("the element answered that the task is faultless after a write it could not " +
			"confirm — which is a false-health answer")
	}
	if got := faults[0].ErrorDescription; !strings.HasPrefix(got, x1.TaskIssueDuplicationNotProgrammed+":") {
		t.Errorf("fault = %q, want one scoped %q", got, x1.TaskIssueDuplicationNotProgrammed)
	}
}

// The boundary the other direction: a push the datapath confirmed — what sessionProgrammed
// records after an accepted write — means the element knows the datapath is duplicating. Nothing about that is an LI condition, and reporting it
// would spend the credibility of the element's most serious report on a non-event.
func TestAConfirmedWriteRaisesNoReport(t *testing.T) {
	f := newEnablerFixture(t)

	before := len(f.reported)
	f.e.recordFARs(108, []far{{farID: 1, fseID: 108, liDuplicate: true}}, true)

	if len(f.reported) != before {
		t.Errorf("reports %d -> %d: a write the datapath accepted was reported as a fault",
			before, len(f.reported))
	}
	if value, held := f.recorded(108, 1); !held || !value {
		t.Errorf("recorded as (duplicating=%v, held=%v); a confirmed push must record what the "+
			"datapath actually holds", value, held)
	}
}
