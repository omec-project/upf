// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"slices"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/omec-project/util/li/x1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// fakeCounters serves whatever the test wants the datapath's accounting to say.
type fakeCounters struct {
	// sent and portDropped are the egress port's output counters: copies written to the
	// shipper, and copies its write did not take.
	sent, portDropped uint64
	// dropped is the buffering stage's own discard count — loss the port never sees,
	// because a queue drops on enqueue and those copies never reach the port.
	dropped uint64
	// noQueue reproduces a pipeline whose egress stage is the merge, which has no status
	// command to ask.
	noQueue bool
	// portErr reproduces an unreachable or renamed port.
	portErr bool
	// traffic is how many copies the datapath sends, losslessly, before each counter read
	// is answered — traffic passing between two reads that are not atomic.
	traffic uint64
	// tees is each tee's gate counts, by module name. A tee not in the map is not in the
	// pipeline.
	tees map[string]*teeGates
	// infoErr makes GetModuleInfo fail at the transport, as a datapath that does not answer.
	infoErr bool
}

// teeGates is one tee's output gates as GetModuleInfo reports them.
type teeGates struct {
	original, copied uint64
	// untrackedCopy reproduces a copy gate without a Track hook -- connected with the default
	// hooks skipped, or with tracking disabled on it -- which bessd reports with no counts,
	// while the original gate still counts.
	untrackedCopy bool
}

func (f *fakeCounters) GetModuleInfo(_ context.Context, in *pb.GetModuleInfoRequest, _ ...grpc.CallOption) (*pb.GetModuleInfoResponse, error) {
	if f.infoErr {
		return nil, context.DeadlineExceeded
	}

	tee, ok := f.tees[in.GetName()]
	if !ok {
		return &pb.GetModuleInfoResponse{Error: &pb.Error{Errmsg: "no such module"}}, nil
	}

	gate := func(idx, pkts uint64) *pb.GetModuleInfoResponse_OGate {
		if idx == teeCopyGate && tee.untrackedCopy {
			return &pb.GetModuleInfoResponse_OGate{Ogate: idx}
		}

		return &pb.GetModuleInfoResponse_OGate{
			Ogate:     idx,
			Pkts:      pkts,
			Gatehooks: []*pb.GetModuleInfoResponse_GateHook{{ClassName: "Track", HookName: "track0"}},
		}
	}

	return &pb.GetModuleInfoResponse{Ogates: []*pb.GetModuleInfoResponse_OGate{
		gate(teeOriginalGate, tee.original),
		gate(teeCopyGate, tee.copied),
	}}, nil
}

// teeMonitor is a monitor over a pipeline whose only readable accounting is the uplink tee.
func teeMonitor(original, copied uint64) (*liPuntMonitor, *teeGates, *recordingReporter) {
	tee := &teeGates{original: original, copied: copied}
	c := &fakeCounters{noQueue: true, portErr: true, tees: map[string]*teeGates{"liReplicaU": tee}}
	rec := &recordingReporter{}

	return &liPuntMonitor{client: c, reporter: rec}, tee, rec
}

func (f *fakeCounters) ModuleCommand(_ context.Context, in *pb.CommandRequest, _ ...grpc.CallOption) (*pb.CommandResponse, error) {
	f.sent += f.traffic
	if f.noQueue || in.GetName() != liQueueModule || in.GetCmd() != "get_status" {
		return &pb.CommandResponse{Error: &pb.Error{Errmsg: "no such module"}}, nil
	}

	data, err := anypb.New(&pb.QueueCommandGetStatusResponse{Dropped: f.dropped})
	if err != nil {
		return nil, err
	}

	return &pb.CommandResponse{Data: data}, nil
}

func (f *fakeCounters) GetPortStats(_ context.Context, _ *pb.GetPortStatsRequest, _ ...grpc.CallOption) (*pb.GetPortStatsResponse, error) {
	f.sent += f.traffic
	if f.portErr {
		return &pb.GetPortStatsResponse{Error: &pb.Error{Errmsg: "no such port"}}, nil
	}

	return &pb.GetPortStatsResponse{Out: &pb.GetPortStatsResponse_Stat{
		Packets: f.sent,
		Dropped: f.portDropped,
	}}, nil
}

// TestPuntMonitorReportsNewLossOnly covers the reporting logic: content discarded
// on the way out of the datapath has to reach the ADMF, and has to do so without
// re-reporting a loss that has stopped growing.
func TestPuntMonitorReportsNewLossOnly(t *testing.T) {
	c := &fakeCounters{sent: 1000, noQueue: true}
	rec := &recordingReporter{}
	m := &liPuntMonitor{client: c, reporter: rec}

	// Nothing discarded: nothing to say.
	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("reported %v with no loss", rec.issues)
	}

	// The port's write starts failing.
	c.sent, c.portDropped = 1900, 100
	m.check()

	if len(rec.issues) != 1 || rec.issues[0] != x1.NEIssueX3PuntLost {
		t.Fatalf("reported %v, want one %s", rec.issues, x1.NEIssueX3PuntLost)
	}

	// Traffic continues with no further discards: already reported, so silent.
	// Re-reporting a static figure on every poll would bury the report that matters in noise.
	c.sent = 2900
	m.check()

	if len(rec.issues) != 1 {
		t.Errorf("reported %v; a discard count that stopped growing was reported again", rec.issues)
	}

	// More copies are discarded: that is new loss and must be reported again.
	c.sent, c.portDropped = 3800, 200
	m.check()

	if len(rec.issues) != 2 {
		t.Errorf("reported %v, want a second report when the loss grew", rec.issues)
	}
}

// TestPuntMonitorHandlesRestartAndUnknowns checks the readings that must not be
// mistaken for good news. Reporting nothing because a counter was unreadable is
// acceptable; reporting nothing because loss was misread as recovery is not.
func TestPuntMonitorHandlesRestartAndUnknowns(t *testing.T) {
	t.Run("the discard count goes down across a restart", func(t *testing.T) {
		c := &fakeCounters{sent: 4000, portDropped: 1000, noQueue: true}
		rec := &recordingReporter{}
		m := &liPuntMonitor{client: c, reporter: rec}

		m.check() // establishes a baseline of 1000 and reports it

		// The datapath restarted: a cumulative count went down, which is impossible within
		// one lifetime, so this is a new baseline rather than a recovery — and not new loss.
		c.sent, c.portDropped = 500, 10
		m.check()

		if len(rec.issues) != 1 {
			t.Fatalf("reported %v after a restart, want only the pre-restart report", rec.issues)
		}

		// The baseline must have been dropped, so the next genuine loss is reported rather
		// than being hidden under the pre-restart figure.
		c.sent, c.portDropped = 950, 60
		m.check()

		if len(rec.issues) != 2 {
			t.Errorf("reported %v, want the pre-restart report and the new one", rec.issues)
		}
	})

	// An unreadable *port* is left to the shipper, which notices its socket has gone
	// and reports x3EgressDown — a second report of the same fact from here would
	// add nothing. And it is not a reading: the baseline stands, so loss that grew
	// across the gap is still reported once the port answers again.
	t.Run("port unreachable or renamed", func(t *testing.T) {
		c := &fakeCounters{sent: 100, portDropped: 50, noQueue: true}
		rec := &recordingReporter{}
		m := &liPuntMonitor{client: c, reporter: rec}

		m.check() // reports the 50

		c.portErr = true
		m.check()

		if len(rec.issues) != 1 {
			t.Fatalf("reported %v; an unreadable port is the shipper's to report", rec.issues)
		}

		c.portErr, c.portDropped = false, 80
		m.check()

		if len(rec.issues) != 2 {
			t.Errorf("reported %v; loss that grew while the port was unreadable was not reported",
				rec.issues)
		}
	})
}

// TestPuntMonitorNeedsAReportingChannel covers the deployment where no ADMF is
// configured. The finding has nowhere legitimate to go — a general log is exactly
// where interception must not appear — so the monitor does not run at all.
func TestPuntMonitorNeedsAReportingChannel(t *testing.T) {
	startLIPuntMonitor(&fakeCounters{}, nil)
	startLIPuntMonitor(nil, &recordingReporter{})
}

// TestPuntMonitorSeesTheQueuesOwnDiscards: the port's discard count is blind to the queue in
// front of it.
//
// A queue discards on *enqueue*, when it is full, so the copies it drops never reach the port
// and the port's counters say nothing about them: during sustained overflow the port sends
// everything it is given and its discard count stays exactly zero while content is being lost.
// The queue's own counter is the one that sees it.
func TestPuntMonitorSeesTheQueuesOwnDiscards(t *testing.T) {
	c := &fakeCounters{sent: 1000}
	rec := &recordingReporter{}
	m := &liPuntMonitor{client: c, reporter: rec}

	m.check()
	if n := len(rec.reported()); n != 0 {
		t.Fatalf("reported %d faults before any loss", n)
	}

	// A burst overflows the queue. The port sends everything the queue dequeued, so its
	// discard count does not move.
	c.sent, c.dropped = 2000, 500

	m.check()

	if !slices.Contains(rec.reported(), x1.NEIssueX3PuntLost) {
		t.Errorf("content was discarded by the egress queue and reported as %v: the port's "+
			"discard count cannot see copies that never reached it", rec.reported())
	}

	// A steady figure is one fault, not a fault per poll.
	before := len(rec.reported())
	m.check()
	if now := len(rec.reported()); now != before {
		t.Errorf("a discard count that stopped growing was reported again (%d → %d)", before, now)
	}
}

// TestPuntMonitorDoesNotMistakeTrafficForLoss is the sampling half.
//
// The counter reads are not atomic, and live interception keeps copies moving between them.
// Inferring loss from two reads of moving packet counters — what was handed to the port
// against what it sent — makes that traffic look like loss on every poll it straddles, so a
// lossless egress would report X3 content loss to the ADMF. A cumulative discard count does
// not move with traffic, only with discards.
func TestPuntMonitorDoesNotMistakeTrafficForLoss(t *testing.T) {
	// Lossless traffic: 200 copies sent before each read is answered, none discarded.
	c := &fakeCounters{sent: 1000, traffic: 200}
	rec := &recordingReporter{}
	m := &liPuntMonitor{client: c, reporter: rec}

	for range 5 {
		m.check()
	}

	if len(rec.issues) != 0 {
		t.Errorf("lossless traffic was reported as %v: copies passing between two counter reads "+
			"are not copies discarded", rec.issues)
	}

	// And real loss in the middle of the same traffic is still seen.
	c.portDropped = 7
	m.check()

	if len(rec.issues) != 1 || rec.issues[0] != x1.NEIssueX3PuntLost {
		t.Errorf("reported %v, want one %s once copies were discarded", rec.issues, x1.NEIssueX3PuntLost)
	}
}

// TestPuntMonitorReportsACopyTheTeeDidNotMake: Replicate skips a copy when the packet pool is
// empty, and nothing downstream of the tee can see it. The original gate getting ahead of the
// copy gate by more than a batch is reported once, and not again while it stays there.
func TestPuntMonitorReportsACopyTheTeeDidNotMake(t *testing.T) {
	m, tee, rec := teeMonitor(1000, 1000)

	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("reported %v for a tee that copied everything", rec.issues)
	}

	tee.original, tee.copied = 2000, 1900
	m.check()

	if len(rec.issues) != 1 || rec.issues[0] != x1.NEIssueX3PuntLost {
		t.Fatalf("100 copies the tee did not make reported %v, want one %q", rec.issues, x1.NEIssueX3PuntLost)
	}

	tee.original, tee.copied = 3000, 2900
	m.check()

	if len(rec.issues) != 1 {
		t.Errorf("a shortfall that stopped growing was reported again: %v", rec.issues)
	}
}

// TestPuntMonitorDoesNotMistakeTeeSkewForLoss: on lossless traffic a read can find the copy
// gate up to a batch ahead of the original, since a batch's copy is counted first.
func TestPuntMonitorDoesNotMistakeTeeSkewForLoss(t *testing.T) {
	m, tee, rec := teeMonitor(0, 0)

	for i, skew := range []uint64{0, 40, 64, 0, 13, 64, 64, 0} {
		base := uint64(1000 * (i + 1))
		tee.original, tee.copied = base, base+skew
		m.check()
	}

	if len(rec.issues) != 0 {
		t.Errorf("lossless traffic read with the copy gate ahead was reported as %v", rec.issues)
	}
}

// TestPuntMonitorReportsGrowthWithinABatchAPollLater: a growth of up to one batch is not
// reported at once. It is not reported when the next poll no longer shows it, and is reported
// on the next poll when that poll still does.
func TestPuntMonitorReportsGrowthWithinABatchAPollLater(t *testing.T) {
	m, tee, rec := teeMonitor(1000, 1000)
	m.check()

	tee.original, tee.copied = 2030, 2000
	m.check()

	tee.original, tee.copied = 3000, 3000
	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("a shortfall within a batch that cleared was reported as %v", rec.issues)
	}

	tee.original, tee.copied = 4030, 4000
	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("a shortfall within a batch was reported before the next poll: %v", rec.issues)
	}

	tee.original, tee.copied = 5030, 5000
	m.check()

	if len(rec.issues) != 1 {
		t.Errorf("a shortfall within a batch that persisted was reported %v, want one", rec.issues)
	}
}

// TestPuntMonitorTakesATeeResetAsANewBaseline: a pipeline reload starts the gate counts from
// zero. That is a new baseline, and loss after it is measured from there.
func TestPuntMonitorTakesATeeResetAsANewBaseline(t *testing.T) {
	m, tee, rec := teeMonitor(1000, 1000)
	m.check()

	tee.original, tee.copied = 5000, 4800
	m.check()

	if len(rec.issues) != 1 {
		t.Fatalf("precondition: the shortfall before the reload was reported %v, want one", rec.issues)
	}

	tee.original, tee.copied = 10, 10
	m.check()

	tee.original, tee.copied = 2000, 2000
	m.check()

	if len(rec.issues) != 1 {
		t.Fatalf("a reload, or lossless traffic after it, was reported: %v", rec.issues)
	}

	tee.original, tee.copied = 3000, 2900
	m.check()

	if len(rec.issues) != 2 {
		t.Errorf("100 copies lost after a reload reported %v, want a second report", rec.issues)
	}
}

// TestPuntMonitorIsSilentWithoutTeeCounts: an older pipeline has no tee by that name, and a
// copy gate without a Track hook reads zero while the original counts. Neither is loss.
func TestPuntMonitorIsSilentWithoutTeeCounts(t *testing.T) {
	m, tee, rec := teeMonitor(5000, 0)
	tee.untrackedCopy = true

	m.check()
	tee.original = 9000
	m.check()

	missing := &liPuntMonitor{client: &fakeCounters{noQueue: true, portErr: true}, reporter: rec}
	missing.check()
	missing.check()

	if len(rec.issues) != 0 {
		t.Errorf("a tee whose gates are not counted, or no tee at all, was reported as %v", rec.issues)
	}
}

// TestPuntMonitorClampsAResetBaselineAtZero: a reload read with the copy gate ahead, then
// lossless reads. The copy-ahead read must not leave a baseline below zero that the lossless
// reads then grow past.
func TestPuntMonitorClampsAResetBaselineAtZero(t *testing.T) {
	m, tee, rec := teeMonitor(5000, 5000)
	m.check()

	tee.original, tee.copied = 100, 130
	m.check()

	for _, n := range []uint64{1000, 2000, 3000} {
		tee.original, tee.copied = n, n
		m.check()
	}

	if len(rec.issues) != 0 {
		t.Errorf("lossless traffic after a reload read with the copy gate ahead was reported as %v",
			rec.issues)
	}
}

// TestPuntMonitorTakesAResetOfOneGateAsANewBaseline: a Track reset of one gate only (bessctl
// can reset a hook) drops that count alone. That is a new baseline, not a loss or a recovery.
func TestPuntMonitorTakesAResetOfOneGateAsANewBaseline(t *testing.T) {
	m, tee, rec := teeMonitor(5000, 5000)
	m.check()

	tee.original, tee.copied = 100, 5100
	m.check()

	tee.original, tee.copied = 1100, 6100
	m.check()
	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("a reset of the original gate alone, then lossless traffic, was reported as %v",
			rec.issues)
	}

	tee.original, tee.copied = 6000, 5100
	m.check()
	m.check()

	if len(rec.issues) != 0 {
		t.Errorf("a reset of the copy gate alone, then lossless traffic, was reported as %v", rec.issues)
	}
}

// TestPuntMonitorKeepsItsAccountThroughAFailedRead: a poll the datapath does not answer
// changes nothing. A shortfall already reported is not reported again after it, and new loss
// after it is.
func TestPuntMonitorKeepsItsAccountThroughAFailedRead(t *testing.T) {
	m, tee, rec := teeMonitor(1000, 1000)
	c := m.client.(*fakeCounters)
	m.check()

	tee.original, tee.copied = 2000, 1900
	m.check()

	c.infoErr = true
	m.check()
	c.infoErr = false

	tee.original, tee.copied = 3000, 2900
	m.check()

	if len(rec.issues) != 1 {
		t.Fatalf("a shortfall reported before a failed read was reported %d times, want once",
			len(rec.issues))
	}

	tee.original, tee.copied = 4000, 3800
	m.check()

	if len(rec.issues) != 2 {
		t.Errorf("new loss after a failed read was reported %v, want a second report", rec.issues)
	}
}

// TestPuntMonitorKeepsTheTwoTeesApart: the downlink and uplink tees are accounted separately,
// so a shortfall on one is neither hidden nor repeated by the other's counts.
func TestPuntMonitorKeepsTheTwoTeesApart(t *testing.T) {
	down := &teeGates{original: 1000, copied: 1000}
	up := &teeGates{original: 50000, copied: 50000}
	c := &fakeCounters{
		noQueue: true,
		portErr: true,
		tees:    map[string]*teeGates{"liReplicaD": down, "liReplicaU": up},
	}
	rec := &recordingReporter{}
	m := &liPuntMonitor{client: c, reporter: rec}

	m.check()

	if len(rec.issues) != 0 {
		t.Fatalf("two lossless tees were reported as %v", rec.issues)
	}

	up.original, up.copied = 60000, 59900
	m.check()
	m.check()

	if len(rec.issues) != 1 {
		t.Fatalf("100 copies the uplink tee did not make reported %v, want once", rec.issues)
	}

	down.original, down.copied = 2000, 1900
	m.check()

	if len(rec.issues) != 2 {
		t.Errorf("100 copies the downlink tee did not make after the uplink's reported %v, want "+
			"a second report", rec.issues)
	}
}
