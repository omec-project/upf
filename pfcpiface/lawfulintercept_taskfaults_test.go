// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"strings"
	"testing"

	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
)

// TestAWorkingTaskReportsNoFault comes first, because a supplier that reports a fault for
// everything satisfies every assertion below and is worthless.
func TestAWorkingTaskReportsNoFault(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the tasked session is not duplicated, so this test would pass for the wrong reason")
	}
	if faults := f.e.taskFaults("W1"); len(faults) != 0 {
		t.Errorf("a task whose traffic is being duplicated reports %d fault(s): %+v — a fault list "+
			"must be evidence of a fault rather than of the element having something to say",
			len(faults), faults)
	}
}

// TestATaskTheDatapathIsNotDuplicatingReportsAFault is the condition this supplier exists for.
//
// The datapath refuses the write, so nothing is duplicated and no copy is made. An element-scoped
// report saying something at this point of interception was refused is not enough: a triggering
// function holding several warrants could not tell which of them had stopped producing.
func TestATaskTheDatapathIsNotDuplicatingReportsAFault(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))

	// The datapath refuses everything from here on.
	f.mu.Lock()
	f.cause = 64 // CauseRequestRejected
	f.mu.Unlock()

	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	if f.duplicates(t, 100, 1) {
		t.Fatal("the fake datapath recorded a write it refused, so this test is asserting against " +
			"a datapath that accepted the rule")
	}

	faults := f.e.taskFaults("W1")
	if len(faults) == 0 {
		t.Fatal("a task whose duplication the datapath refused reports no fault, so a triggering " +
			"function is told the task is provisioned and carrying no faults while it produces nothing")
	}
	if !strings.Contains(faults[0].ErrorDescription, "not duplicating") {
		t.Errorf("the fault does not say what is wrong: %q", faults[0].ErrorDescription)
	}
	if faults[0].ErrorCode == 0 {
		t.Error("the fault carries no issue code, so it cannot be correlated with the pushed report")
	}
}

// TestATaskFaultIsAttributedToOneWarrant: two warrants, one session each, and only one of them
// refused. An answer that named both would be no more useful to a triggering function than an
// element-scoped report.
func TestATaskFaultIsAttributedToOneWarrant(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	if !f.duplicates(t, 100, 1) {
		t.Fatal("the first warrant's session is not duplicated")
	}

	// Now the datapath starts refusing, and a second warrant arrives.
	f.putSession(t, unmarkedSession(200, "10.250.0.10"))
	f.mu.Lock()
	f.cause = 64
	f.mu.Unlock()
	f.activate(t, "W2", ueAddr("10.250.0.10"))
	f.settle(t)

	if got := f.e.taskFaults("W2"); len(got) == 0 {
		t.Error("the refused warrant reports no fault")
	}
	if got := f.e.taskFaults("W1"); len(got) != 0 {
		t.Errorf("the warrant that is working reports %+v; a triggering function acting on this "+
			"would report triggerFaulty for a warrant whose interception is running", got)
	}
}

// TestATaskSelectingNoSessionSaysSo covers the other re-observable condition. A CC-POI's task is
// installed against traffic that exists, so a task selecting nothing is an interception
// producing nothing — which is exactly what a triggering function has no other way to learn,
// since a copy that is never made leaves no record anywhere.
func TestATaskSelectingNoSessionSaysSo(t *testing.T) {
	f := newEnablerFixture(t)
	f.activate(t, "W1", ueAddr(testUEIPv4))

	faults := f.e.taskFaults("W1")
	if len(faults) == 0 {
		t.Fatal("a task no session carries traffic for reports nothing, so the element answers " +
			"that it is provisioned and has no faults while intercepting nothing")
	}
	if !strings.Contains(faults[0].ErrorDescription, "producing nothing") {
		t.Errorf("the fault does not say what was observed: %q", faults[0].ErrorDescription)
	}

	// And it clears by itself when the session appears: the answer is computed, so nothing has
	// to remember to retract it. This is the half a stored fault gets wrong.
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.e.retaskAndWait()
	f.settle(t)
	if got := f.e.taskFaults("W1"); len(got) != 0 {
		t.Errorf("the fault outlived the condition: %+v", got)
	}
}

// TestATaskFaultNamesNoSubject: the answer is scoped to the task, so it already identifies the
// warrant. The description is the only free-form field in it, and a criterion is a subject
// identifier — an address, a TEID, a SEID — so an implementation that quoted the criterion it
// resolved would put the subject on the X1 interface.
func TestATaskFaultNamesNoSubject(t *testing.T) {
	const ueIP = testUEIPv4

	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, ueIP))
	f.mu.Lock()
	f.cause = 64
	f.mu.Unlock()
	f.activate(t, "W1", ueAddr(ueIP))
	f.settle(t)

	faults := f.e.taskFaults("W1")
	if len(faults) == 0 {
		t.Fatal("no fault to check")
	}
	for _, fault := range faults {
		if strings.Contains(fault.ErrorDescription, ueIP) {
			t.Errorf("the fault names the subject's address (%q): %q", ueIP, fault.ErrorDescription)
		}
		if strings.Contains(fault.ErrorDescription, "100") {
			t.Errorf("the fault names the session identifier: %q", fault.ErrorDescription)
		}
	}
}

// TestATaskTheElementDoesNotHoldReportsNothing: a fault answered for an unknown XID would be a
// claim about a warrant this element was never given.
func TestATaskTheElementDoesNotHoldReportsNothing(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4))

	if got := f.e.taskFaults(types.XID("W-never-installed")); len(got) != 0 {
		t.Errorf("the element answered %+v about a task it does not hold", got)
	}
}

// TestATaskSelectsTrafficOnlyWhereEveryCriterionHolds: a task's criteria are conjunctive (TS
// 103 221-1 table 6.2.1.2-1). This session's UE address matches the task's, but the task also
// names a PFCP session this element does not hold, so the task selects nothing — and must say so.
// Read per criterion, the address would select the session and the task would report itself
// healthy while producing nothing.
func TestATaskSelectsTrafficOnlyWhereEveryCriterionHolds(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W1", ueAddr(testUEIPv4),
		types.TargetIdentifier{Type: types.TargetFSEID, Value: "999"})
	f.settle(t)

	faults := f.e.taskFaults("W1")
	if len(faults) != 1 || !strings.Contains(faults[0].ErrorDescription, "carries traffic this task selects") {
		t.Errorf("a task whose criteria no session satisfies together reports %+v, want a "+
			"no-traffic-selected fault", faults)
	}
}

// TestATaskWithACriterionThatDoesNotParseIsLeftOutWhole: criteria are conjunctive, so dropping
// the one that does not parse and keeping the rest would widen the task to traffic its warrant
// does not describe. The whole task is left out instead.
func TestATaskWithACriterionThatDoesNotParseIsLeftOutWhole(t *testing.T) {
	good := ueAddr(testUEIPv4)
	bad := types.TargetIdentifier{Type: types.TargetTCPPort, Value: "not-a-port"}

	if _, ok := parseAll(types.InterceptTask{Targets: []types.TargetIdentifier{good}}); !ok {
		t.Fatal("a task whose every criterion parses was left out")
	}
	if set, ok := parseAll(types.InterceptTask{Targets: []types.TargetIdentifier{good, bad}}); ok {
		t.Errorf("a task with a criterion that does not parse was kept, narrowed to %d criteria", len(set))
	}
}

// hasCondition reports whether a task's fault list carries the named condition, which
// TaskFault puts at the head of the description.
func hasCondition(faults []x1.X1Error, condition string) bool {
	for _, f := range faults {
		if strings.HasPrefix(f.ErrorDescription, condition+": ") {
			return true
		}
	}

	return false
}

// TestAWarrantServedNothingByAnOverlapSaysSo: two warrants select one session, and the shipping
// path delivers each copy under the lowest XID only. The other warrant is authorised and receives
// none of that session's content, which nothing in the delivered product would show — so it is
// that warrant, and only that one, that answers with a fault. The fault is a state: once the
// overlap ends it is no longer answered, which is what lets the task watcher retract it.
func TestAWarrantServedNothingByAnOverlapSaysSo(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W-b", ueAddr(testUEIPv4))
	f.activate(t, "W-a", types.TargetIdentifier{Type: types.TargetFSEID, Value: "100"})
	f.settle(t)

	if task, _, ok := lookupTrigger(f.tasks, f.e, 100); !ok || task.XID != "W-a" {
		t.Fatalf("the session's content goes to %q, want W-a, so this test would assert the "+
			"wrong warrant", task.XID)
	}

	faults := f.e.taskFaults("W-b")
	if !hasCondition(faults, x1.TaskIssueContentUnderAnotherTask) {
		t.Fatalf("the warrant receiving none of the session's content answers %+v, so the ADMF "+
			"is told it is faultless while its product is incomplete", faults)
	}
	if len(faults) != 1 {
		t.Errorf("the overlapping warrant answers %d faults, want only the overlap: %+v", len(faults), faults)
	}
	if !strings.Contains(faults[0].ErrorDescription, "1 session(s)") {
		t.Errorf("the fault does not count the sessions delivered elsewhere: %q", faults[0].ErrorDescription)
	}
	if got := f.e.taskFaults("W-a"); len(got) != 0 {
		t.Errorf("the warrant receiving the content answers %+v; nothing is wrong with it", got)
	}

	// The warrant ahead of it goes, so the session's content is now W-b's, and the fault with it.
	f.deactivate(t, "W-a")
	f.settle(t)
	if got := f.e.taskFaults("W-b"); hasCondition(got, x1.TaskIssueContentUnderAnotherTask) {
		t.Errorf("the overlap fault outlived the overlap: %+v", got)
	}
}

// TestAnOverlapAndARefusalAreBothAnswered: a warrant whose content goes to another task and whose
// duplication the datapath could not confirm has two conditions, each with its own ending.
// Answering only one would leave the other to be discovered only once the first ended.
func TestAnOverlapAndARefusalAreBothAnswered(t *testing.T) {
	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, "W-b", ueAddr(testUEIPv4))
	f.activate(t, "W-a", types.TargetIdentifier{Type: types.TargetFSEID, Value: "100"})
	f.settle(t)

	// The session's rules were written and the write not confirmed, so the record no longer
	// claims they are duplicating.
	f.e.farsAttempted(100, []far{{farID: 1, fseID: 100, liDuplicate: true}})

	got := f.e.taskFaults("W-b")
	if !hasCondition(got, x1.TaskIssueContentUnderAnotherTask) ||
		!hasCondition(got, x1.TaskIssueDuplicationNotProgrammed) {
		t.Errorf("W-b answers %+v, want both duplicationNotProgrammed and contentUnderAnotherTask", got)
	}
	if got := f.e.taskFaults("W-a"); hasCondition(got, x1.TaskIssueContentUnderAnotherTask) {
		t.Errorf("W-a receives the content but answers %+v", got)
	}
}

// TestAPartialOverlapSaysTheContentIsNotAllDelivered: the warrant ahead selects only one direction
// of the session's tunnel, while the other selects the whole session. The shipping path takes the
// first covering task and applies that task's filter alone, so the copies it rejects are dropped
// rather than offered to the next task: the second warrant receives nothing, and part of the
// session reaches nobody. The fault must say that, rather than claim the content went elsewhere.
func TestAPartialOverlapSaysTheContentIsNotAllDelivered(t *testing.T) {
	t.Parallel()

	const ahead types.XID = "W-a"

	f := newEnablerFixture(t)
	f.putSession(t, unmarkedSession(100, testUEIPv4))
	f.activate(t, ahead, types.TargetIdentifier{Type: types.TargetGTPTunnelDirection, Value: x1.GTPDirectionOutbound})
	f.activate(t, "W-b", types.TargetIdentifier{Type: types.TargetFSEID, Value: "100"})
	f.settle(t)

	task, filter, ok := lookupTrigger(f.tasks, f.e, 100)
	if !ok || task.XID != ahead {
		t.Fatalf("the session's content goes to %q, want %q", task.XID, ahead)
	}
	up := filter.matches(farForwardUAndDuplicate, uplinkCopy(443))
	down := filter.matches(farForwardDAndDuplicate, downlinkCopy(443))
	if up == down {
		t.Fatalf("the winning filter takes uplink=%v downlink=%v, so this is not a partial overlap "+
			"and the test would assert nothing about dropped copies", up, down)
	}

	faults := f.e.taskFaults("W-b")
	if !hasCondition(faults, x1.TaskIssueContentUnderAnotherTask) {
		t.Fatalf("the whole-session warrant answers %+v, want contentUnderAnotherTask", faults)
	}
	var desc string
	for _, fl := range faults {
		if strings.HasPrefix(fl.ErrorDescription, x1.TaskIssueContentUnderAnotherTask+": ") {
			desc = fl.ErrorDescription
		}
	}
	for _, want := range []string{"1 session(s)", "lower-XID task", "or not at all"} {
		if !strings.Contains(desc, want) {
			t.Errorf("the overlap fault %q does not say %q: a partial overlap drops what the first "+
				"task's criteria exclude, so 'delivered under another task' alone is untrue", desc, want)
		}
	}
}
