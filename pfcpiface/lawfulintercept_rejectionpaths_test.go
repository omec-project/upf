// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// liRefusingDP is a datapath that refuses what it is asked to do *after* applying it, which is what
// the real one does: SendMsgToUPF issues one gRPC call per rule and GRPCJoin returns on the first
// failure with the rest of the batch still in flight, so a "rejected" answer means an unknown
// subset was applied — not that nothing was.
//
// Every other datapath fake in this package models all-or-nothing, which is why none of them can
// reach the state the live incident produced.
type liRefusingDP struct {
	fakeDP

	mu     sync.Mutex
	refuse bool
	// refuseDelete refuses only deletion batches, so a modification's write can be
	// accepted and its deletion stage refused -- the branch the modification handler's rollback runs on.
	refuseDelete bool
	// unconfirmed models the third answer: the datapath applied nothing it refused, but
	// could not say whether it applied what it accepted. Every other fake here is
	// all-or-nothing, which is why none of them can reach the state an RPC that did not
	// complete produces.
	unconfirmed bool
	// unfinishedDelete makes every deletion batch report that it did not finish, as one that
	// ran out of time does: the answer on which the establishment rollback stores the session.
	unfinishedDelete bool
	methods          []upfMsgType
	// duplicating is what this datapath believes it is duplicating, keyed by FAR ID. It is
	// written whatever the answer, because that is the point.
	duplicating map[uint32]bool
	// everDuplicating is every FAR a write asked to duplicate, kept through deletions.
	everDuplicating map[uint32]bool
}

// sharedMetrics builds the collector once for the whole package.
//
// The session handlers record per-session metrics, so the embedded collector must be real: it is
// an interface field and a nil one panics inside NewPFCPSession, before the handler reaches
// anything these tests are about. It cannot be per-test, because the constructor registers global
// Prometheus collectors and the second call fails.
var sharedMetrics = sync.OnceValues(func() (*metrics.Service, error) {
	return metrics.NewPrometheusService()
})

func newRefusingDP() *liRefusingDP {
	return &liRefusingDP{duplicating: make(map[uint32]bool), everDuplicating: make(map[uint32]bool)}
}

func (d *liRefusingDP) SendMsgToUPF(method upfMsgType, all, updated PacketForwardingRules) uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.methods = append(d.methods, method)

	rules := updated
	if method == upfMsgTypeDel || len(updated.fars) == 0 {
		rules = all
	}

	for _, f := range rules.fars {
		if method == upfMsgTypeDel {
			delete(d.duplicating, f.farID)
			continue
		}

		d.duplicating[f.farID] = f.liDuplicate
		if f.liDuplicate {
			d.everDuplicating[f.farID] = true
		}
	}

	if d.refuse || (d.refuseDelete && method == upfMsgTypeDel) {
		return ie.CauseRequestRejected
	}

	return ie.CauseRequestAccepted
}

// SendMsgToUPFWithCompletion routes through SendMsgToUPF: the embedded fakeDP's version would
// not reach the override above, because embedding does not dispatch. Every batch finished
// unless unfinishedDelete says a deletion did not.
func (d *liRefusingDP) SendMsgToUPFWithCompletion(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	cause := d.SendMsgToUPF(method, all, updated)

	d.mu.Lock()
	defer d.mu.Unlock()

	return cause, !d.unfinishedDelete || method != upfMsgTypeDel
}

// sendMsgToUPFConfirmed makes this fake a confirmingDatapath, so a test can drive the
// accepted-but-unconfirmed branch through the real handlers.
func (d *liRefusingDP) sendMsgToUPFConfirmed(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	cause := d.SendMsgToUPF(method, all, updated)

	d.mu.Lock()
	defer d.mu.Unlock()

	return cause, cause == ie.CauseRequestAccepted && !d.unconfirmed
}

// sawDuplicating reports whether the last write of a FAR asked the datapath to duplicate it, or,
// once it was deleted, whether any did. Deletion clears what duplicating holds, so the history is
// kept alongside it.
func (d *liRefusingDP) sawDuplicating(farID uint32) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.duplicating[farID] || d.everDuplicating[farID]
}

func (d *liRefusingDP) sawMethod(m upfMsgType) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, seen := range d.methods {
		if seen == m {
			return true
		}
	}

	return false
}

// rejectionConn builds a connection whose datapath refuses, wired to a real enabler as production
// wires it — without the enabler, applyTasking and every recorder are nil-receiver no-ops and the
// paths under test record nothing for reasons unrelated to the defect.
func rejectionConn(t *testing.T) (*PFCPConn, *liRefusingDP, *ccEnabler, *[]string) {
	t.Helper()

	pConn, dp, e, reported, _ := rejectionConnDescribed(t)

	return pConn, dp, e, reported
}

// rejectionConnDescribed is rejectionConn that also keeps each report's description, for the
// tests where what a report says is the point and not only its issue code.
func rejectionConnDescribed(t *testing.T) (*PFCPConn, *liRefusingDP, *ccEnabler, *[]string, *[]string) {
	t.Helper()

	dp := newRefusingDP()
	sessions := NewInMemoryStore()
	tasks := store.New()

	svc, err := sharedMetrics()
	if err != nil {
		t.Fatalf("metrics service: %v", err)
	}

	var (
		mu        sync.Mutex
		reported  []string
		described []string
	)

	// The enabler's own passes push through the same confirmation the handlers see, as in
	// production. With a push that confirmed every accepted write, the pass a handler asks for
	// recorded as duplicating a write the datapath never acknowledged, whenever it ran after
	// the handler's own record.
	e := newCCEnabler(tasks, func(all, updated PacketForwardingRules) (uint8, bool) {
		return dp.sendMsgToUPFConfirmed(upfMsgTypeMod, all, updated)
	}, func(issueType, description string) {
		mu.Lock()
		defer mu.Unlock()

		reported = append(reported, issueType)
		described = append(described, description)
	})

	t.Cleanup(e.stop)
	e.addSource(sessions)

	pConn := &PFCPConn{
		Conn:  nil,
		store: sessions,
		// localIE is what the handlers put in every response they build; a nil one panics
		// while the response is assembled, before the assertion is reached.
		nodeID: nodeID{remote: "1.1.1.1", local: "2.2.2.2", localIE: ie.NewNodeID("2.2.2.2", "", "")},
		upf:    &upf{datapath: dp, ccEnabler: e},
		// The establishment handler allocates a local SEID from these, as production does.
		// Fixed seed: the SEID is not asserted on, and a deterministic test is worth more than
		// an arbitrary one.
		rng:            rand.New(rand.NewSource(1)), // #nosec G404
		maxRetries:     100,
		InstrumentPFCP: svc,
	}

	return pConn, dp, e, &reported, &described
}

// programmedFor reports what the element's record says about a FAR, and whether it holds one.
// caller happens to assert on FAR 1 today, and a helper that hard-coded it would stop
// being able to ask the question these tests are about.
//
//nolint:unparam // farID is the key under test, not a constant of the helper: every
func programmedFor(e *ccEnabler, seid uint64, farID uint32) (bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry, held := e.programmed[farRef{seid: seid, farID: farID}]

	return entry.duplicating, held
}

// TestARefusedModificationRecordsWhatItPushed drives the real handler rather than calling
// the recorder directly.
//
// That distinction matters. A test that calls the recorder by hand passes with three of the four
// call sites missing — the remedy present at the deletion stage, say, and absent from the branch
// that reaches the datapath first.
//
// Without the record, a later re-derivation computes "nothing should duplicate", finds no entry,
// reads that as "not duplicating", and concludes there is nothing to turn off. The copies continue
// under no authority, and the element's own account is what conceals it.
func TestARefusedModificationRecordsWhatItPushed(t *testing.T) {
	pConn, dp, e, _ := rejectionConn(t)

	const seid = 400

	sess := liStoredSession(seid, "10.250.0.12")
	if err := pConn.store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	dp.mu.Lock()
	dp.refuse = true
	dp.mu.Unlock()

	// The SMF restates FAR 1. The datapath applies it and then refuses the batch.
	_, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, seid, 1, 0,
			ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
			),
		))
	if err == nil {
		t.Fatal("the refused modification was reported as succeeding")
	}

	if _, held := programmedFor(e, seid, 1); !held {
		t.Error("a refused modification left no record of the FAR it had already pushed. The " +
			"datapath may be duplicating it, and no re-derivation can turn that off, because " +
			"the element's record says there is nothing to turn off")
	}
}

// TestARefusedEstablishmentDoesNotStrandDuplication drives the establishment handler.
//
// This path cannot be fixed by recording: NewPFCPSession does not store the session and, when
// the rollback finishes, the branch abandons it before PutSession, so no re-derivation will ever
// walk it and no record written here could be read again. Removing the rules from the datapath is the only remedy, and
// the absence of that push is what leaves a PDR matching the UE address and a FAR duplicating for
// the life of the process.
func TestARefusedEstablishmentDoesNotStrandDuplication(t *testing.T) {
	pConn, dp, _, _ := rejectionConn(t)

	dp.mu.Lock()
	dp.refuse = true
	dp.mu.Unlock()

	ueIP := net.ParseIP("10.250.0.12")

	_, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
			ie.NewNodeID("1.1.1.1", "", ""),
			ie.NewFSEID(200, net.ParseIP("1.1.1.1"), nil),
			ie.NewCreatePDR(
				ie.NewPDRID(1),
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, ueIP.String(), "", 0, 0),
				),
				ie.NewFARID(1),
			),
			ie.NewCreateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
			),
		))
	if err == nil {
		t.Fatal("the refused establishment was reported as succeeding")
	}

	if !dp.sawMethod(upfMsgTypeDel) {
		t.Error("a refused establishment abandoned the session without asking the datapath to " +
			"remove the rules it had already applied. The session is never stored, so nothing " +
			"in this element will ever walk it again: whatever was programmed — including " +
			"duplication — stays for the life of the process, with no record of it anywhere")
	}
}

// TestARefusedEstablishmentOfATaskedSessionIsReported drives the abandoning report on the
// establishment path: a warrant already covers the UE, so the refused write carried duplication,
// the rollback that should have removed it was refused too, and the session is never stored. Nothing
// in this element will walk it again, so saying so is the only remedy left.
func TestARefusedEstablishmentOfATaskedSessionIsReported(t *testing.T) {
	pConn, dp, e, _, described := rejectionConnDescribed(t)

	const ue = "10.250.0.16"

	if !e.tasks.Activate(types.InterceptTask{
		XID:      testXIDPrimary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{ueAddr(ue)},
	}) {
		t.Fatal("Activate failed")
	}

	e.reparse()

	// Every batch refused, the rollback's deletion included, and every batch finished: the
	// branch that abandons the session rather than storing it.
	dp.mu.Lock()
	dp.refuse = true
	dp.mu.Unlock()

	_, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
			ie.NewNodeID("1.1.1.1", "", ""),
			ie.NewFSEID(202, net.ParseIP("1.1.1.1"), nil),
			ie.NewCreatePDR(
				ie.NewPDRID(1),
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, ue, "", 0, 0),
				),
				ie.NewFARID(1),
			),
			ie.NewCreateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
			),
		))
	if err == nil {
		t.Fatal("the refused establishment was reported as succeeding")
	}

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("the handler holds %d sessions; this test is about the branch that abandons it", n)
	}

	for _, d := range *described {
		if strings.Contains(d, "did not accept the removal of a duplication rule") &&
			strings.Contains(d, "a refused session establishment") {
			return
		}
	}

	t.Errorf("a refused establishment of a tasked session, whose rollback was refused too, was "+
		"abandoned without saying so: its duplication may still be running with nothing able to "+
		"stop it. Reported: %v", *described)
}

// TestARefusedEstablishmentWhoseRollbackDidNotFinishIsRecorded drives the other half of the
// establishment's refusal: the rollback's removal runs out of time, so the handler stores the
// session after all rather than forget rules nothing else names.
//
// Stored, the session is one a re-derivation walks, and that changes what the record must say.
// Neither the refused add nor the unfinished removal says whether FAR 1 is duplicating, so the
// record has to hold it as attempted: an entry claiming duplication would agree with the tasking
// and be skipped by every later pass, and no entry at all leaves a withdrawal with nothing it
// knows to turn off.
func TestARefusedEstablishmentWhoseRollbackDidNotFinishIsRecorded(t *testing.T) {
	pConn, dp, e, reported := rejectionConn(t)

	const ue = "10.250.0.15"

	if !e.tasks.Activate(types.InterceptTask{
		XID:      testXIDSecondary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{ueAddr(ue)},
	}) {
		t.Fatal("Activate failed")
	}

	e.reparse()

	dp.mu.Lock()
	dp.refuse = true
	dp.unfinishedDelete = true
	dp.mu.Unlock()

	_, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
			ie.NewNodeID("1.1.1.1", "", ""),
			ie.NewFSEID(201, net.ParseIP("1.1.1.1"), nil),
			ie.NewCreatePDR(
				ie.NewPDRID(1),
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, ue, "", 0, 0),
				),
				ie.NewFARID(1),
			),
			ie.NewCreateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
			),
		))
	if err == nil {
		t.Fatal("the refused establishment was reported as succeeding")
	}

	stored := pConn.store.GetAllSessions()
	if len(stored) != 1 {
		t.Fatalf("the handler holds %d sessions, want the one whose rollback did not finish; "+
			"this test is not exercising the branch it exists for", len(stored))
	}

	duplicating, held := programmedFor(e, stored[0].localSEID, 1)
	if !held {
		t.Error("a stored session's FARs have no record, so a withdrawal of the task finds " +
			"nothing it knows to turn off on a session whose duplication may be running")
	}

	if duplicating {
		t.Error("a FAR the datapath refused was recorded as duplicating: the record agrees " +
			"with the tasking, so no later re-derivation pushes it again")
	}

	if len(*reported) == 0 || (*reported)[0] != x1.NEIssueDuplicationRefused {
		t.Errorf("reported %v, want %q: the ADMF is not told that an accepted task's "+
			"duplication may not be running", *reported, x1.NEIssueDuplicationRefused)
	}
}

// TestARefusedLocalDeleteKeepsItsRecord covers the session-report-response path, which must not
// delete the record before it knows whether the datapath accepted the removal.
//
// The record's entries go with the session, so dropping them first means a refused delete strands
// rules that no later re-derivation can find — it walks the sessions this element holds, and this
// one is gone.
func TestARefusedLocalDeleteKeepsItsRecord(t *testing.T) {
	pConn, dp, e, _ := rejectionConn(t)

	const seid = 401

	sess := liStoredSession(seid, "10.250.0.13")
	// RemoveSession reads the session's metrics, so without them a delete that ran in the
	// wrong order would panic rather than fail.
	sess.metrics = &metrics.Session{}
	if err := pConn.store.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	// The element believes FAR 1 is duplicating.
	e.recordFARs(seid, []far{{farID: 1, fseID: seid, liDuplicate: true}}, true)

	if dup, held := programmedFor(e, seid, 1); !held || !dup {
		t.Fatalf("fixture did not establish the record: duplicating=%v held=%v", dup, held)
	}

	dp.mu.Lock()
	dp.refuse = true
	dp.mu.Unlock()

	err := pConn.handleSessionReportResponse(
		message.NewSessionReportResponse(0, 0, seid, 1, 0,
			ie.NewCause(ie.CauseSessionContextNotFound),
		))
	if err == nil {
		t.Fatal("the refused local delete was reported as succeeding")
	}

	if _, held := programmedFor(e, seid, 1); !held {
		t.Error("the record was dropped before the datapath confirmed the removal, so a rule " +
			"the datapath still holds is now invisible to every re-derivation")
	}
}

// TestAbandonedDuplicationIsReportedOnlyWhenSomethingWasDuplicating covers the report the two
// abandoning paths raise — a refused establishment, and an association teardown.
//
// Those are the paths where recording is not a remedy: the session is gone or was never stored, so
// no re-derivation can ever walk it. Saying so is all that is left, and it must be said only when
// it means something. An ordinary failed delete of a session no warrant covered is the datapath's
// business; reporting that to the ADMF would manufacture LI faults out of routine PFCP churn, and
// an element that always reports is ignored exactly as fast as one that never does.
func TestAbandonedDuplicationIsReportedOnlyWhenSomethingWasDuplicating(t *testing.T) {
	for _, tt := range []struct {
		name string
		fars []far
		want bool
	}{
		{"a rule that was duplicating", []far{{farID: 1, liDuplicate: true}}, true},
		{"one of several duplicating", []far{{farID: 1}, {farID: 2, liDuplicate: true}}, true},
		{"nothing was duplicating", []far{{farID: 1}, {farID: 2}}, false},
		{"no rules at all", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, e, reported := rejectionConn(t)

			e.abandonedDuplication(1, tt.fars, "a test")

			got := len(*reported) > 0
			if got != tt.want {
				t.Errorf("reported=%v want=%v (%v)", got, tt.want, *reported)
			}

			if tt.want && (*reported)[0] != x1.NEIssueDuplicationRefused {
				t.Errorf("reported %q, want %q", (*reported)[0], x1.NEIssueDuplicationRefused)
			}
		})
	}
}

// TestAPartiallyProgrammedRefusalIsStillWithdrawn is the whole shape in one test, against a
// datapath that behaves like the real one.
//
// A warrant is active and the element programs duplication. The datapath then refuses a pass
// *after* applying it — which is what SendMsgToUPF does, because GRPCJoin returns on the first
// failure with the rest of the batch still in flight. The warrant is withdrawn. Duplication must
// stop.
//
// The state to avoid is one where it never stops: the refusal leaves the record saying "not
// duplicating" while the datapath duplicates, and `transact` skips any FAR whose record already
// equals what the tasking implies. The element computes "nothing should duplicate", sees its
// record agreeing, and does nothing — for ever. `everDuplicated` bounds that, but only for a FAR
// the element recorded as duplicating in the first place, which a refusal must therefore not
// prevent.
//
// The fixture's all-or-nothing mode cannot reach this state at all: with `partial` off, a refused push programs nothing and the record and the datapath never
// disagree. See enablerFixture.partial.
func TestAPartiallyProgrammedRefusalIsStillWithdrawn(t *testing.T) {
	f := newEnablerFixture(t)

	const seid = 500

	f.putSession(t, unmarkedSession(seid, "10.250.0.21"))

	// The datapath applies what it is told and then refuses the batch.
	f.mu.Lock()
	f.partial = true
	f.cause = ie.CauseRequestRejected
	f.mu.Unlock()

	f.activate(t, "W1", ueAddr("10.250.0.21"))

	if !f.duplicates(t, seid, 1) {
		t.Fatal("the datapath did not start duplicating, so this test is not exercising the " +
			"state it exists for")
	}

	// The datapath recovers, and the warrant ends.
	f.mu.Lock()
	f.cause = ie.CauseRequestAccepted
	f.mu.Unlock()

	f.deactivate(t, "W1")

	if f.duplicates(t, seid, 1) {
		t.Error("duplication continued after the warrant was withdrawn. The refusal left the " +
			"element's record disagreeing with the datapath, and a re-derivation that trusts " +
			"that record concludes there is nothing to turn off — so a subscriber's traffic is " +
			"copied under no authority, invisibly from both ends")
	}
}

// TestEverDuplicatedIsReclaimedWithItsSessions pins the second record against the same requirement
// as the first.
//
// Every record of what was programmed must stay bounded, and this set is one. Keeping an entry
// after its session ends buys nothing: transact consults the set only inside its walk of live
// sessions' FARs, and a later session cannot inherit the entry by reusing the SEID, because
// NewPFCPSession allocates from a 64-bit random source.
//
// Nor is the set small. A task keyed by network instance or tunnel direction selects every
// session on the element, so under ordinary subscriber churn an unpruned set would grow with
// every session that has ever existed while the task was held. Interception would stay correct
// the whole time, which is what makes it invisible — until the process dies and takes every
// warrant with it.
//
// sessionForgotten is called directly here because the churn is what is under test; its production
// call site is RemoveSession, which the rejection-path tests in this file drive.
func TestEverDuplicatedIsReclaimedWithItsSessions(t *testing.T) {
	f := newEnablerFixture(t)
	f.activate(t, "W1", ueAddr(testUEIPv4))

	const rounds = 200

	for seid := uint64(1); seid <= rounds; seid++ {
		s := unmarkedSession(seid, testUEIPv4)
		f.putSession(t, s)

		// ...and the subscriber goes away again, as RemoveSession does in production.
		if err := f.store.DeleteSession(seid); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}

		f.e.sessionForgotten(&s)
	}

	f.e.retaskAndWait()
	f.settle(t)

	programmed, everDuplicated := f.recordSizes()

	if programmed != 0 {
		t.Errorf("the primary record holds %d entries with no sessions left", programmed)
	}

	if everDuplicated != 0 {
		t.Errorf("the ever-duplicated set holds %d entries with no sessions left. It is a record "+
			"of what was programmed and is bounded by the same requirement; every one of those "+
			"entries is unreachable, because transact consults the set only while walking the "+
			"FARs of a live session", everDuplicated)
	}
}

// TestAFARRecreatedUnderTheSameIDKeepsItsRecord pins the deletion sweep against the entry's own
// stamp rather than the pass's mark.
//
// Comparing to the mark answers "was this forgotten after the pass began", which also discards an
// entry written *after* the forgetting — and a FAR re-created under the same identifier is exactly
// that. A path switch does it routinely: the modification removes FAR n, stamping it forgotten,
// then re-creates FAR n and records it duplicating. Both stamps are past the mark, so a sweep
// against the mark would drop a record describing a live, duplicating rule.
//
// A missing entry reads as "not duplicating", so until the next pass this element would answer
// TaskIssueDuplicationNotProgrammed for a warrant it is in fact serving — a wrong answer rather
// than lost interception, and the answer the triggering function acts on.
func TestAFARRecreatedUnderTheSameIDKeepsItsRecord(t *testing.T) {
	f := newEnablerFixture(t)

	const seid = 600

	sess := unmarkedSession(seid, testUEIPv4)
	f.putSession(t, sess)
	w := f.windowed(t)

	task := ccTask("W1", ueAddr(testUEIPv4))
	if err := f.e.canApply(task); err != nil {
		t.Fatalf("canApply: %v", err)
	}

	if !f.tasks.Activate(task) {
		t.Fatal("Activate failed")
	}

	// A pass begins and reads the sessions.
	w.hold <- struct{}{}
	gen := f.e.request()
	<-w.read

	// Inside the interval the SMF removes FAR 1 and immediately re-creates it — one path
	// switch, as far as this element is concerned.
	f.e.farsRemoved(seid, []far{{farID: 1, fseID: seid}})
	f.e.recordFARs(seid, []far{{farID: 1, fseID: seid, liDuplicate: true}}, true)

	// The pass concludes on its older view.
	w.release <- struct{}{}
	f.e.await(gen)
	f.settle(t)

	value, held := f.recorded(seid, 1)
	if !held {
		t.Fatal("the record for a FAR re-created after it was forgotten was swept away. It is " +
			"live and duplicating; a missing entry reads as 'not duplicating', so this element " +
			"now reports the interception as not programmed for a warrant it is serving")
	}

	if !value {
		t.Error("the record says the re-created FAR is not duplicating, while the element pushed " +
			"it duplicating")
	}
}

// TestConcurrentMissesShareOneFragmentMemo pins the memoisation against the one thing that makes
// two equivalent answers *not* interchangeable.
//
// Resolution runs unlocked — it walks every session, so it must — and the framing workers are four.
// Two of them missing on the same session at the same time both compute an answer, and the answers
// agree about which tasks cover the session. What they do not share is the fragment memo, which
// resolveCovering allocates: it remembers what the initial fragment of a datagram was classified
// as, so the later fragments, which carry no transport header, are delivered or dropped with it.
//
// Last-writer-wins left the losing worker holding a filter whose memo nothing else shares. A
// datagram whose initial fragment was classified into one memo and whose later fragments were
// looked up in the other loses its tail — dropped, and reported as X3 delivery loss, for content
// the criterion did select. A manufactured loss report is worse than a silent one: it tells the
// agency product went missing that never did.
func TestConcurrentMissesShareOneFragmentMemo(t *testing.T) {
	f := newEnablerFixture(t)

	const seid = 700

	// A transport-port criterion on rules that constrain no port: the only shape whose filter
	// has to read the packet, and therefore the only one that carries a fragment memo at all.
	// A UE-address criterion yields a match-all filter with no memo, so it cannot exercise this.
	f.putSession(t, unmarkedSession(seid, testUEIPv4))
	f.activate(t, "W1", types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"})

	// Force every worker to miss: a tasking change invalidates the memo by epoch.
	f.e.epoch.Add(1)

	const workers = 8

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		answers [][]coveredTask
	)

	start := make(chan struct{})

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			got := f.e.tasksCovering(seid)

			mu.Lock()
			answers = append(answers, got)
			mu.Unlock()
		}()
	}

	close(start)
	wg.Wait()

	var memo *fragmentMemo

	for i, got := range answers {
		if len(got) == 0 {
			t.Fatalf("worker %d resolved no covering task; the fixture is not exercising a miss", i)
		}

		if got[0].filter.frags == nil {
			t.Fatalf("worker %d resolved a filter with no fragment memo, so this test compares "+
				"nil against nil and would pass against the defect", i)
		}

		if memo == nil {
			memo = got[0].filter.frags

			continue
		}

		if got[0].filter.frags != memo {
			t.Fatalf("worker %d holds a different fragment memo from the others. A datagram whose "+
				"initial fragment is classified into one memo and whose later fragments are looked "+
				"up in another loses its tail — dropped, and reported as delivery loss, for "+
				"content the criterion selected", i)
		}
	}
}

// countingStore reports how many times the sessions were walked, which is the cost taskFaults must
// not pay on the X1 request goroutine for an answer nothing has invalidated.
type countingStore struct {
	SessionsStore

	mu    sync.Mutex
	walks int
}

func (c *countingStore) GetAllSessions() []PFCPSession {
	c.mu.Lock()
	c.walks++
	c.mu.Unlock()

	return c.SessionsStore.GetAllSessions()
}

func (c *countingStore) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.walks
}

// TestTaskFaultsDoesNotWalkEverySessionPerRequest pins the cost off the X1 request goroutine.
//
// li/x1 asks this once per task on *every* response that carries tasks — an activation, a
// modification, a deactivation, a status query alike. Answering it from scratch resolves every
// criterion against every session this element holds, so an element with twenty thousand sessions
// and ten tasks would resolve two hundred thousand criterion sets to answer one provisioning
// message, on the goroutine answering it.
//
// That is exactly the cost the enabler's worker exists to keep off the request path — its own
// doc-comment says "an X1 request's latency would become a function of session count on the
// element holding the most sessions".
//
// The answer is still determined when asked rather than pushed: the memo is stamped with the
// tasking epoch and the record's write counter, and every event that could change the answer moves
// one of them. What is avoided is recomputing an answer nothing has invalidated.
func TestTaskFaultsDoesNotWalkEverySessionPerRequest(t *testing.T) {
	f := newEnablerFixture(t)

	counting := &countingStore{SessionsStore: f.store}
	f.e.mu.Lock()
	f.e.sources = []SessionsStore{counting}
	f.e.mu.Unlock()

	for seid := uint64(800); seid < 810; seid++ {
		f.putSession(t, unmarkedSession(seid, testUEIPv4))
	}

	f.activate(t, "W1", ueAddr(testUEIPv4))
	f.settle(t)

	// The first ask resolves.
	_ = f.e.taskFaults("W1")

	after := counting.count()

	// Nine more, with nothing changed in between: an ADMF interrogating, or simply several
	// X1 responses each carrying the task.
	for range 9 {
		_ = f.e.taskFaults("W1")
	}

	if grew := counting.count() - after; grew != 0 {
		t.Errorf("nine further X1 answers walked the sessions %d more times with nothing changed "+
			"between them. An X1 request's latency is then a function of session count, which is "+
			"the cost the enabler's worker exists to keep off this goroutine", grew)
	}

	// And a tasking change invalidates it, so the answer is still the current one rather than
	// a cached history.
	before := counting.count()

	f.deactivate(t, "W1")
	_ = f.e.taskFaults("W1")

	if counting.count() == before {
		t.Error("a tasking change did not invalidate the answer; a status reply that survives the " +
			"tasking it describes is a history, not a state")
	}
}

// TestAnUnconfirmedModificationDoesNotRecordDuplicationAsProgrammed drives the real
// handler, which is the only way to establish that the handler passes the datapath's
// confirmation on rather than its cause.
//
// The datapath accepts the batch and cannot say whether it applied it -- an RPC that did
// not complete, which SendMsgToUPF answers as accepted on purpose. If the handler recorded
// that as programmed, the FAR would match the tasking on every later pass, drop out of the
// difference, and never be pushed again: an accepted warrant producing nothing, with this
// element's own record concealing it. ETSI TS 103 221-1 clause 5.3 puts the obligation the
// other way -- an issue that loses traffic is a fault the element must hold and report.
func TestAnUnconfirmedModificationDoesNotRecordDuplicationAsProgrammed(t *testing.T) {
	pConn, dp, e, _ := rejectionConn(t)

	const seid = 402

	if err := pConn.store.PutSession(liStoredSession(seid, "10.250.0.14")); err != nil {
		t.Fatal(err)
	}

	// A warrant covering the session, so applyTasking marks its FARs and there is a
	// duplication claim for the record to get right or wrong.
	if !e.tasks.Activate(types.InterceptTask{
		XID:      testXIDSecondary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{ueAddr("10.250.0.14")},
	}) {
		t.Fatal("Activate failed")
	}

	e.reparse()

	dp.mu.Lock()
	dp.unconfirmed = true
	dp.mu.Unlock()

	if _, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, seid, 1, 0,
			ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
			),
		)); err != nil {
		t.Fatalf("the modification was refused: %v", err)
	}

	duplicating, held := programmedFor(e, seid, 1)
	if !held {
		t.Fatal("the modification left no record of the FAR it pushed")
	}

	if duplicating {
		t.Error("a write the datapath never acknowledged was recorded as duplicating: the FAR " +
			"now agrees with the tasking, so every later re-derivation skips it and nothing " +
			"ever pushes it again, while an interrogation of the task reports no fault")
	}
}

// TestARefusedDeletionStageDoesNotClaimTheRolledBackDuplication drives a modification whose
// write the datapath accepts and whose deletion stage it refuses.
//
// The refusal makes the handler roll the datapath back to the rules the session had, and those
// predate the tasking this message applied: the restore turns FAR 1's duplication off again.
// Recording the pushed rules as duplicating would then claim a copy the datapath is not making,
// and because that claim agrees with the tasking, every later re-derivation would skip the FAR --
// the warrant producing nothing while the element reports it healthy.
func TestARefusedDeletionStageDoesNotClaimTheRolledBackDuplication(t *testing.T) {
	pConn, dp, e, _ := rejectionConn(t)

	const seid = 403

	if err := pConn.store.PutSession(liStoredSession(seid, "10.250.0.15")); err != nil {
		t.Fatal(err)
	}

	if !e.tasks.Activate(types.InterceptTask{
		XID:      testXIDSecondary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{ueAddr("10.250.0.15")},
	}) {
		t.Fatal("Activate failed")
	}

	e.reparse()

	dp.mu.Lock()
	dp.refuseDelete = true
	dp.mu.Unlock()

	if _, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, seid, 1, 0,
			ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
			),
			ie.NewRemoveFAR(ie.NewFARID(4)),
		)); err == nil {
		t.Fatal("a modification whose deletion stage was refused was reported as succeeding")
	}

	dp.mu.Lock()
	restored := !dp.duplicating[1]
	dp.mu.Unlock()

	if !restored {
		t.Fatal("the rollback did not restore FAR 1 to the session's own, non-duplicating " +
			"version, so this test is not exercising the state it is about")
	}

	if duplicating, _ := programmedFor(e, seid, 1); duplicating {
		t.Error("the record claims FAR 1 is duplicating after the rollback turned it off: the " +
			"claim agrees with the tasking, so no later re-derivation pushes it again")
	}
}

// TestARefusedRemovalAfterThePushDoesNotClaimTheRolledBackDuplication drives a modification
// whose write the datapath accepts and which then fails on its own content: it removes a FAR
// the session does not hold.
//
// The failure comes after the push, so the handler rolls the datapath back to the session's
// stored rules, and for a session tasked after establishment those do not duplicate. The pass
// before the modification recorded FAR 1 as duplicating; if the handler leaves that record
// standing, it agrees with the tasking and every later re-derivation skips the FAR while the
// datapath copies nothing.
//
// FAR 2 is the case that is easy to miss: the message does not name it, but the rollback
// re-writes every FAR the session held, so it is turned off too and its record must say so.
func TestARefusedRemovalAfterThePushDoesNotClaimTheRolledBackDuplication(t *testing.T) {
	pConn, dp, e, _ := rejectionConn(t)

	const seid = 404

	if err := pConn.store.PutSession(liStoredSession(seid, "10.250.0.16")); err != nil {
		t.Fatal(err)
	}

	if !e.tasks.Activate(types.InterceptTask{
		XID:      testXIDSecondary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{ueAddr("10.250.0.16")},
	}) {
		t.Fatal("Activate failed")
	}

	e.reparse()
	e.retaskAndWait()

	for _, id := range []uint32{1, 2} {
		if duplicating, _ := programmedFor(e, seid, id); !duplicating {
			t.Fatalf("the tasking pass did not record FAR %d as duplicating, so this test is not "+
				"exercising the state it is about", id)
		}
	}

	if _, err := pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, seid, 1, 0,
			ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
			),
			ie.NewRemoveFAR(ie.NewFARID(99)),
		)); err == nil {
		t.Fatal("a modification removing a FAR the session does not hold was reported as succeeding")
	}

	for _, id := range []uint32{1, 2} {
		dp.mu.Lock()
		restored := !dp.duplicating[id]
		dp.mu.Unlock()

		if !restored {
			t.Fatalf("the rollback did not restore FAR %d to the session's own, non-duplicating "+
				"version, so this test is not exercising the state it is about", id)
		}

		if duplicating, _ := programmedFor(e, seid, id); duplicating {
			t.Errorf("the record claims FAR %d is duplicating after the rollback turned it off: the "+
				"claim agrees with the tasking, so no later re-derivation pushes it again", id)
		}
	}
}
