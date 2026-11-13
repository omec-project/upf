// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// A URR that leaves its session -- with the session, or removed by a modification --
// reports what it measured since its last report in the response (TS 29.244 clause
// 5.2.2.3.1), with the TERMR trigger (clause 8.2.41).

const idleURRID = testURRID + 1

// quietURR measures volume and has no trigger that fires: its usage is reported only
// when it leaves the session.
func quietURR(id uint32) *ie.IE {
	return ie.NewCreateURR(
		ie.NewURRID(id),
		ie.NewMeasurementMethod(0, 1, 0),
		ie.NewReportingTriggers(triggerOctet5PERIO, 0), // no Measurement Period, so it never fires
	)
}

// newFinalUsageSetup is a reportingSetup whose session has a URR measuring its downlink
// PDR and an idle one.
func newFinalUsageSetup(t *testing.T) *reportingSetup {
	t.Helper()

	rs := newIdleReportingSetup(t)

	if _, err := rs.pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, rs.localSEID, 1, 0)); err != nil {
		t.Fatalf("could not clear the setup's session: %v", err)
	}

	rs.localSEID = establishStaticSession(t, rs.pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID)), downlinkFAR(),
		quietURR(testURRID), quietURR(idleURRID))

	// Clearing the setup's session drained; the test's own drain must read.
	rs.monitor.lastRead = time.Time{}

	return rs
}

func (rs *reportingSetup) deleteSession(t *testing.T, seq uint32) *message.SessionDeletionResponse {
	t.Helper()

	rsp, err := rs.pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, rs.localSEID, seq, 0))
	if err != nil {
		t.Fatalf("deletion refused: %v", err)
	}

	sdres, ok := rsp.(*message.SessionDeletionResponse)
	if !ok {
		t.Fatalf("expected a Session Deletion Response, got %T", rsp)
	}

	assertAccepted(t, sdres.Cause, "deletion")

	return sdres
}

// reportsByURR reads usage reports, checking that each is a termination report.
func reportsByURR(t *testing.T, reports []*ie.IE) map[uint32][]reportedUsage {
	t.Helper()

	got := map[uint32][]reportedUsage{}

	for _, r := range reports {
		u := readUsageReport(t, r)
		got[u.urrID] = append(got[u.urrID], u)
	}

	return got
}

func isTermination(u reportedUsage) bool {
	return len(u.trigger) >= 2 && u.trigger[1]&uint8(triggerTERMR>>8) != 0
}

// Every URR of a deleted session reports, an idle one with zero volume (clause 5.2.2.3.1).
func TestADeletionReportsTheFinalUsageOfEveryURR(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	got := reportsByURR(t, rs.deleteSession(t, 2).UsageReport)

	measured, idle := got[testURRID], got[idleURRID]
	if len(measured) != 1 || len(idle) != 1 {
		t.Fatalf("expected one report per URR, got %+v", got)
	}

	for _, u := range []reportedUsage{measured[0], idle[0]} {
		if !isTermination(u) || u.seq != 0 {
			t.Fatalf("URR %d reported trigger %x with UR-SEQN %d, expected TERMR with UR-SEQN 0",
				u.urrID, u.trigger, u.seq)
		}
	}

	if measured[0].volume.TotalVolume != 2*mb || measured[0].volume.DownlinkVolume != 2*mb {
		t.Fatalf("the final report carries %+v, expected the 2 MB the datapath counted since the last poll",
			measured[0].volume)
	}

	if idle[0].volume == nil || idle[0].volume.TotalVolume != 0 {
		t.Fatalf("the idle URR's final report carries %+v, expected zero volume", idle[0].volume)
	}
}

// refusingDeleteDP refuses every deletion.
type refusingDeleteDP struct{ fakeDP }

func (*refusingDeleteDP) SendMsgToUPFWithCompletion(method upfMsgType, _, _ PacketForwardingRules) (uint8, bool) {
	if method == upfMsgTypeDel {
		return ie.CauseRequestRejected, true
	}

	return ie.CauseRequestAccepted, true
}

// A refused deletion keeps the session, and its usage with it: the control plane's retry
// reports it.
func TestARefusedDeletionKeepsTheUsageForTheRetry(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))
	rs.pConn.upf.datapath = &refusingDeleteDP{}

	if _, err := rs.pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, rs.localSEID, 2, 0)); err == nil {
		t.Fatal("the deletion was accepted, so this test proves nothing")
	}

	rs.pConn.upf.datapath = &fakeDP{}

	got := reportsByURR(t, rs.deleteSession(t, 3).UsageReport)
	if m := got[testURRID]; len(m) != 1 || m[0].volume.TotalVolume != 2*mb || m[0].seq != 0 {
		t.Fatalf("the retry reports %+v, expected the 2 MB with UR-SEQN 0", m)
	}
}

// A control plane that lost the deletion response retransmits the request; it gets the
// response again, final usage and all, and not "Session context not found".
func TestARetransmittedDeletionIsAnsweredWithItsResponse(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	first := rs.deleteSession(t, 2)
	again := rs.deleteSession(t, 2)

	if len(again.UsageReport) != len(first.UsageReport) {
		t.Fatalf("the retransmission was answered with %d usage reports, expected the %d of the original",
			len(again.UsageReport), len(first.UsageReport))
	}
}

// A report that may not have reached the control plane goes in the final response too,
// with its UR-SEQN, ahead of the termination report.
func TestADeletionCarriesTheOutstandingReport(t *testing.T) {
	// The setup's URR has a 1000-byte threshold. With no sender, its report stays
	// outstanding.
	rs := newIdleReportingSetup(t)
	rs.monitor.batches = make(chan reportBatch)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	rs.reader.set(downlinkCounts(rs.localSEID, 300))
	rs.monitor.lastRead = time.Time{} // the deletion's drain reads

	got := reportsByURR(t, rs.deleteSession(t, 2).UsageReport)[testURRID]
	if len(got) != 2 {
		t.Fatalf("expected the outstanding report and a termination report, got %+v", got)
	}

	if got[0].seq != 0 || got[0].volume.TotalVolume != 1500 || isTermination(got[0]) {
		t.Fatalf("the first report is %+v, expected the outstanding threshold report of 1500 bytes", got[0])
	}

	if got[1].seq != 1 || got[1].volume.TotalVolume != 300 || !isTermination(got[1]) {
		t.Fatalf("the second report is %+v, expected a termination report of the 300 bytes since", got[1])
	}
}

func (rs *reportingSetup) modify(t *testing.T, ies ...*ie.IE) *message.SessionModificationResponse {
	t.Helper()

	rsp, err := rs.pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, rs.localSEID, 2, 0, ies...))
	if err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	smres, ok := rsp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", rsp)
	}

	assertAccepted(t, smres.Cause, "modification")

	return smres
}

// A URR removed by a modification reports its final usage in the response (clause
// 5.2.2.3.1).
func TestRemovingAURRReportsItsFinalUsage(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	smres := rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))

	got := reportsByURR(t, smres.UsageReport)
	if len(got) != 1 || len(got[testURRID]) != 1 {
		t.Fatalf("expected one report, for the removed URR, got %+v", got)
	}

	if r := got[testURRID][0]; !isTermination(r) || r.volume.TotalVolume != 2*mb {
		t.Fatalf("the removed URR reported %+v, expected a termination report of 2 MB", r)
	}
}

// A refused modification reports nothing and removes nothing: the URR and its usage stay
// for the control plane's retry.
func TestARefusedModificationKeepsTheURRItWouldHaveRemoved(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	refuseModification(t, rs.pConn, rs.localSEID, errNotFound,
		ie.NewRemoveURR(ie.NewURRID(testURRID)),
		ie.NewRemoveURR(ie.NewURRID(testURRID+99)),
	)

	smres := rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))
	if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].volume.TotalVolume != 2*mb {
		t.Fatalf("the retry reports %+v, expected the 2 MB", r)
	}
}

// countingReader counts the reads made of it.
type countingReader struct{ reads int }

func (r *countingReader) ReadUsage(context.Context) (flowMeasureCounts, error) {
	r.reads++

	return flowMeasureCounts{preQos: flowMeasureTable{}, postUL: flowMeasureTable{}, postDL: flowMeasureTable{}}, nil
}

// A deletion storm shares reads: a drain accepts a read begun under drainCoalesce before
// it, instead of reading the whole datapath once per deletion.
func TestDrainsShareARecentRead(t *testing.T) {
	reader := &countingReader{}
	m := newUsageMonitor(&PFCPNode{ctx: context.Background()}, reader, time.Hour)

	m.drain()
	m.drain()

	if reader.reads != 1 {
		t.Fatalf("two drains in a row read %d times, expected once", reader.reads)
	}

	m.lastRead = time.Now().Add(-drainCoalesce)
	m.drain()

	if reader.reads != 2 {
		t.Fatalf("a drain after the coalescing window did not read")
	}
}

// A PDR's counts are credited to the URRs that measure it. A modification that removes the
// PDR has them credited first, or they could no longer be: the URR, which stays for the
// session's other rule, still reports them.
func TestRemovingAPDRKeepsTheUsageItCountedForItsURR(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.localSEID = establishStaticSession(t, rs.pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID)),
		corePDR(secondPDRID, secondUEAddress, ie.NewURRID(testURRID)),
		downlinkFAR(), quietURR(testURRID))
	rs.monitor.lastRead = time.Time{}

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb)) // counted on the PDR being removed

	if smres := rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID))); len(smres.UsageReport) != 0 {
		t.Fatal("a URR another PDR still measures reported on the removal of one")
	}

	smres := rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))
	if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].volume.TotalVolume != 2*mb {
		t.Fatalf("the URR reported %+v, expected the 2 MB its removed PDR counted", r)
	}
}

// failingReader is a datapath whose reads fail, after reading the modules in got.
type failingReader struct {
	got   flowMeasureCounts
	reads int
}

func (r *failingReader) ReadUsage(context.Context) (flowMeasureCounts, error) {
	r.reads++

	return r.got, errors.New("bessd unavailable")
}

// A read that did not read both post-QoS modules has not credited what a deleted session
// counted in the other, so it does not stand in for a drain.
func TestAFailedReadIsNotSharedByADrain(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  flowMeasureCounts
	}{
		{"nothing read", flowMeasureCounts{}},
		{"only the uplink read", flowMeasureCounts{postUL: flowMeasureTable{}}},
		{"only the downlink read", flowMeasureCounts{postDL: flowMeasureTable{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &failingReader{got: tc.got}
			m := newUsageMonitor(&PFCPNode{ctx: context.Background()}, reader, time.Hour)

			m.drain()
			m.drain()

			if reader.reads != 2 {
				t.Fatalf("a drain after a failed read read %d times in all, expected it to read again",
					reader.reads)
			}
		})
	}
}

// A URR a modification leaves without a PDR -- its last PDR removed, or pointed at another
// URR -- reports what it measured in the response with TERMR, and is kept with its
// measurement reset (clauses 5.2.2.3.1 and 8.2.41).
func TestAURRThatLosesItsLastPDRReportsItsFinalUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		ies  []*ie.IE
	}{
		{"Remove PDR", []*ie.IE{ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID))}},
		{"Update PDR to another URR", []*ie.IE{ie.NewUpdatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(establishedPrecedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, firstUEAddress, "", 0, 0),
			),
			ie.NewFARID(downlinkFARID),
			ie.NewURRID(idleURRID),
		)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := newFinalUsageSetup(t)

			rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

			got := reportsByURR(t, rs.modify(t, tc.ies...).UsageReport)
			if len(got) != 1 || len(got[testURRID]) != 1 {
				t.Fatalf("expected one report, for the URR that lost its PDR, got %+v", got)
			}

			if r := got[testURRID][0]; !isTermination(r) || r.seq != 0 || r.volume.TotalVolume != 2*mb {
				t.Fatalf("the URR reported %+v, expected a termination report of 2 MB with UR-SEQN 0", r)
			}

			// Kept, and reset: its removal reports nothing more, under the next UR-SEQN.
			smres := rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))
			if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].seq != 1 ||
				r[0].volume.TotalVolume != 0 {
				t.Fatalf("the URR's removal reported %+v, expected zero volume with UR-SEQN 1", r)
			}
		})
	}
}

// A URR that loses its last PDR with nothing measured reports nothing (clause 5.2.2.3.1:
// only non-null measurements), and its UR-SEQN is not used.
func TestAURRThatLosesItsLastPDRWithNothingMeasuredDoesNotReport(t *testing.T) {
	rs := newFinalUsageSetup(t)

	if smres := rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID))); len(smres.UsageReport) != 0 {
		t.Fatalf("a URR with nothing measured reported %d times", len(smres.UsageReport))
	}

	smres := rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))
	if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].seq != 0 {
		t.Fatalf("the URR's removal reported %+v, expected UR-SEQN 0", r)
	}
}

// Only the URRs this modification leaves without a PDR report. One no PDR measured before
// it -- here holding usage a refused report gave back -- does not.
func TestAURRWithoutAPDRBeforeTheModificationDoesNotReport(t *testing.T) {
	rs := newFinalUsageSetup(t)

	s := storedSession(t, rs.pConn, rs.localSEID)
	s.urrs = slices.Clone(s.urrs)
	s.findURR(idleURRID).usage.unreported = downlink(500)

	if err := rs.pConn.store.PutSession(s); err != nil {
		t.Fatal(err)
	}

	smres := rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)))
	if r := reportsByURR(t, smres.UsageReport)[idleURRID]; len(r) != 0 {
		t.Fatalf("a URR no PDR measured before the modification reported %+v", r)
	}
}

// A refused modification reports nothing and resets nothing: the control plane's retry
// reports the usage.
func TestARefusedModificationKeepsTheUsageOfAURRThatWouldLoseItsLastPDR(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	// The modification's second write is the removal.
	rs.pConn.upf.datapath = &refuseNthWriteDP{n: 2}
	refuseModification(t, rs.pConn, rs.localSEID, ErrWriteToDatapath,
		ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)))

	rs.pConn.upf.datapath = &fakeDP{}

	smres := rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)))
	if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].volume.TotalVolume != 2*mb {
		t.Fatalf("the retry reported %+v, expected the 2 MB", r)
	}
}

// A message that removes a URR's PDR and creates another that measures it does not leave
// it without a PDR: it does not report, and keeps its usage.
func TestAURRAnotherNewPDRMeasuresDoesNotReport(t *testing.T) {
	rs := newFinalUsageSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 2*mb))

	smres := rs.modify(t,
		ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)),
		corePDR(secondPDRID, secondUEAddress, ie.NewURRID(testURRID)),
	)
	if len(smres.UsageReport) != 0 {
		t.Fatalf("a URR a new PDR measures reported %d times", len(smres.UsageReport))
	}

	smres = rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))
	if r := reportsByURR(t, smres.UsageReport)[testURRID]; len(r) != 1 || r[0].volume.TotalVolume != 2*mb {
		t.Fatalf("the URR's removal reported %+v, expected the 2 MB", r)
	}
}

// A URR that loses its last PDR with a report outstanding sends it in the response ahead
// of its termination report.
func TestAURRThatLosesItsLastPDRSendsItsOutstandingReportFirst(t *testing.T) {
	// The setup's URR has a 1000-byte threshold. With no sender, its report stays
	// outstanding.
	rs := newIdleReportingSetup(t)
	rs.monitor.batches = make(chan reportBatch)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	rs.reader.set(downlinkCounts(rs.localSEID, 300))
	rs.monitor.lastRead = time.Time{} // the modification's drain reads

	got := reportsByURR(t, rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID))).UsageReport)[testURRID]
	if len(got) != 2 {
		t.Fatalf("expected the outstanding report and a termination report, got %+v", got)
	}

	if got[0].seq != 0 || got[0].volume.TotalVolume != 1500 || isTermination(got[0]) {
		t.Fatalf("the first report is %+v, expected the outstanding threshold report of 1500 bytes", got[0])
	}

	if got[1].seq != 1 || got[1].volume.TotalVolume != 300 || !isTermination(got[1]) {
		t.Fatalf("the second report is %+v, expected a termination report of the 300 bytes since", got[1])
	}
}

// A modification of a session that has no URRs does not read the datapath.
func TestAModificationOfASessionWithoutURRsDoesNotDrain(t *testing.T) {
	rs := newIdleReportingSetup(t)

	reader := &countingReader{}
	rs.monitor.reader = reader

	rs.localSEID = establishStaticSession(t, rs.pConn,
		corePDR(secondPDRID, secondUEAddress), downlinkFAR())
	rs.monitor.lastRead = time.Time{}

	rs.modify(t, ie.NewRemovePDR(ie.NewPDRID(secondPDRID)))

	if reader.reads != 0 {
		t.Fatalf("the modification read the datapath %d times", reader.reads)
	}
}
