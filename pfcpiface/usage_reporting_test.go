// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// Usage reports from the poll to the control plane: a poll credits what the datapath
// counted and sends the reports that fall due in a Session Report Request, which a peer
// standing in for the SMF receives and answers.

// scriptedReader is a datapath whose reads return counts the test sets.
type scriptedReader struct {
	mu     sync.Mutex
	counts flowMeasureCounts
}

func (r *scriptedReader) set(c flowMeasureCounts) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counts = c
}

func (r *scriptedReader) ReadUsage(context.Context) (flowMeasureCounts, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c := r.counts
	r.counts = flowMeasureCounts{}

	return c, nil
}

// downlinkCounts is a read in which the session's downlink PDR forwarded bytes.
func downlinkCounts(localSEID, bytes uint64) flowMeasureCounts {
	key := flowMeasureKey{localSEID, downlinkPDRID}
	st := &pb.FlowMeasureReadResponse_Statistic{Fseid: localSEID, Pdr: downlinkPDRID, TotalBytes: bytes, TotalPackets: 1}

	return flowMeasureCounts{
		preQos: flowMeasureTable{key: st},
		postUL: flowMeasureTable{},
		postDL: flowMeasureTable{key: st},
	}
}

// reportingSetup is an association with a reporting session, its node's usage monitor,
// and the socket of the peer it reports to.
type reportingSetup struct {
	node      *PFCPNode
	pConn     *PFCPConn
	monitor   *usageMonitor
	reader    *scriptedReader
	peer      *net.UDPConn
	localSEID uint64
}

func newReportingSetup(t *testing.T) *reportingSetup {
	t.Helper()

	rs := newIdleReportingSetup(t)

	for range reportSenders {
		go rs.monitor.sendReports()
	}

	return rs
}

// newIdleReportingSetup is a reportingSetup whose monitor has no senders running.
func newIdleReportingSetup(t *testing.T) *reportingSetup {
	t.Helper()

	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { peer.Close() }) //nolint:errcheck // test cleanup

	c, err := net.DialUDP("udp", nil, peer.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { c.Close() }) //nolint:errcheck // test cleanup

	pConn := reportingConn(t)
	pConn.Conn = c
	pConn.upf.maxReqRetries = 1
	pConn.upf.respTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node := &PFCPNode{ctx: ctx, upf: pConn.upf}
	node.pConns.Store("smf", pConn)

	reader := &scriptedReader{}
	node.usage = newUsageMonitor(node, reader, time.Hour)

	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID)), downlinkFAR(),
		volumeURR(testURRID)) // 1000-byte volume threshold

	return &reportingSetup{node, pConn, node.usage, reader, peer, localSEID}
}

// receive reads the next Session Report Request the association sends.
func (rs *reportingSetup) receive(t *testing.T) (*message.SessionReportRequest, *net.UDPAddr) {
	t.Helper()

	buf := make([]byte, 4096)

	if err := rs.peer.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	n, from, err := rs.peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("no Session Report Request: %v", err)
	}

	msg, err := message.Parse(buf[:n])
	if err != nil {
		t.Fatal(err)
	}

	srreq, ok := msg.(*message.SessionReportRequest)
	if !ok {
		t.Fatalf("expected a Session Report Request, got %T", msg)
	}

	return srreq, from
}

// answer hands the association the peer's response, as its reader would.
func (rs *reportingSetup) answer(t *testing.T, srreq *message.SessionReportRequest, cause uint8) {
	t.Helper()

	rsp := message.NewSessionReportResponse(0, 0, 0, srreq.SequenceNumber, 0, ie.NewCause(cause))

	buf := make([]byte, rsp.MarshalLen())
	if err := rsp.MarshalTo(buf); err != nil {
		t.Fatal(err)
	}

	rs.pConn.HandlePFCPMsg(buf)
}

// settled waits until the session's URR has no report in flight.
func (rs *reportingSetup) settled(t *testing.T) urrUsage {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		rs.pConn.sessionsMu.Lock()
		s, _ := rs.pConn.store.GetSession(rs.localSEID)
		rs.pConn.sessionsMu.Unlock()

		if !s.urrs[0].usage.inFlight {
			return s.urrs[0].usage
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("the report did not settle")

	return urrUsage{}
}

type reportedUsage struct {
	urrID   uint32
	seq     uint32
	trigger []uint8
	volume  *ie.VolumeMeasurementFields
	start   time.Time
	end     time.Time
}

func readUsageReport(t *testing.T, report *ie.IE) reportedUsage {
	t.Helper()

	var (
		got reportedUsage
		err error
	)

	children, err := report.UsageReport()
	if err != nil {
		t.Fatal(err)
	}

	for _, x := range children {
		switch x.Type {
		case ie.URRID:
			got.urrID, err = x.URRID()
		case ie.URSEQN:
			got.seq, err = x.URSEQN()
		case ie.UsageReportTrigger:
			got.trigger = x.Payload
		case ie.VolumeMeasurement:
			got.volume, err = x.VolumeMeasurement()
		case ie.StartTime:
			got.start, err = x.StartTime()
		case ie.EndTime:
			got.end, err = x.EndTime()
		}

		if err != nil {
			t.Fatal(err)
		}
	}

	return got
}

func TestAPollSendsTheReportsThatFellDue(t *testing.T) {
	rs := newReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	srreq, _ := rs.receive(t)

	if srreq.SEID() != testRemoteSEID {
		t.Fatalf("the request is addressed to SEID %#x, expected %#x", srreq.SEID(), uint64(testRemoteSEID))
	}

	if srreq.ReportType == nil || !srreq.ReportType.HasUSAR() {
		t.Fatal("the request's Report Type does not say it carries usage reports")
	}

	if len(srreq.UsageReport) != 1 {
		t.Fatalf("expected one usage report, got %d", len(srreq.UsageReport))
	}

	got := readUsageReport(t, srreq.UsageReport[0])

	if got.urrID != testURRID || got.seq != 0 {
		t.Fatalf("the report is for URR %d with UR-SEQN %d, expected URR %d with UR-SEQN 0",
			got.urrID, got.seq, testURRID)
	}

	if len(got.trigger) < 1 || got.trigger[0]&uint8(triggerVOLTH) == 0 {
		t.Fatalf("the report's trigger is %x, expected VOLTH", got.trigger)
	}

	if got.volume == nil || !got.volume.HasTOVOL() || !got.volume.HasULVOL() || !got.volume.HasDLVOL() ||
		got.volume.TotalVolume != 1500 || got.volume.UplinkVolume != 0 || got.volume.DownlinkVolume != 1500 {
		t.Fatalf("the report's volume is %+v, expected all three counters: 1500, 0 and 1500", got.volume)
	}

	if got.start.IsZero() || got.end.IsZero() {
		t.Fatal("the report has no Start Time or End Time")
	}

	rs.answer(t, srreq, ie.CauseRequestAccepted)

	if us := rs.settled(t); us.outstanding != nil {
		t.Fatal("the accepted report is still outstanding")
	}
}

// A report that got no answer is sent again at the next poll with its UR-SEQN and volume,
// and the usage measured meanwhile waits for the report after it.
func TestAReportWithNoAnswerIsSentAgainAtTheNextPoll(t *testing.T) {
	rs := newReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	first, _ := rs.receive(t)
	retransmitted, _ := rs.receive(t) // max_req_retries is 1

	if retransmitted.SequenceNumber != first.SequenceNumber {
		t.Fatalf("the retransmission has sequence number %d, expected the original %d",
			retransmitted.SequenceNumber, first.SequenceNumber)
	}

	rs.settled(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 700))
	rs.monitor.poll(time.Now())

	again, _ := rs.receive(t)
	if len(again.UsageReport) != 1 {
		t.Fatalf("expected the one outstanding report, got %d", len(again.UsageReport))
	}

	if got, want := readUsageReport(t, again.UsageReport[0]), readUsageReport(t, first.UsageReport[0]); got.seq != want.seq ||
		got.volume.TotalVolume != want.volume.TotalVolume {
		t.Fatalf("the report was sent again as UR-SEQN %d with %d bytes, expected UR-SEQN %d with %d",
			got.seq, got.volume.TotalVolume, want.seq, want.volume.TotalVolume)
	}

	rs.answer(t, again, ie.CauseRequestAccepted)
	rs.settled(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 400))
	rs.monitor.poll(time.Now())

	next, _ := rs.receive(t)
	if got := readUsageReport(t, next.UsageReport[0]); got.seq != 1 || got.volume.TotalVolume != 1100 {
		t.Fatalf("the next report is UR-SEQN %d with %d bytes, expected UR-SEQN 1 with the 1100 "+
			"bytes measured while the first was outstanding", got.seq, got.volume.TotalVolume)
	}
}

// A report that finds every sender busy and the queue full is not dropped: it stays
// outstanding and goes out at the next poll.
func TestAReportThatFindsTheQueueFullIsSentAtTheNextPoll(t *testing.T) {
	rs := newIdleReportingSetup(t)
	rs.monitor.batches = make(chan reportBatch, 1)
	rs.monitor.batches <- reportBatch{} // the queue is full, and no sender is running

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	us := rs.settled(t)
	if us.outstanding == nil || us.outstanding.seq != 0 {
		t.Fatalf("the report that found the queue full is not outstanding: %+v", us.outstanding)
	}

	<-rs.monitor.batches // room for one

	rs.monitor.poll(time.Now())

	go rs.monitor.sendReports()

	srreq, _ := rs.receive(t)
	if got := readUsageReport(t, srreq.UsageReport[0]); got.seq != 0 || got.volume.TotalVolume != 1500 {
		t.Fatalf("expected UR-SEQN 0 with 1500 bytes, got UR-SEQN %d with %d", got.seq, got.volume.TotalVolume)
	}
}

// A control plane that refuses a report counted none of it: its usage goes in the next
// report, which has the next UR-SEQN.
func TestARefusedReportsUsageIsSentInTheNextReport(t *testing.T) {
	rs := newReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	refused, _ := rs.receive(t)
	rs.answer(t, refused, ie.CauseRequestRejected)
	rs.settled(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 100))
	rs.monitor.poll(time.Now())

	next, _ := rs.receive(t)
	if got := readUsageReport(t, next.UsageReport[0]); got.seq != 1 || got.volume.TotalVolume != 1600 {
		t.Fatalf("the next report is UR-SEQN %d with %d bytes, expected UR-SEQN 1 with the refused "+
			"1500 bytes and the 100 since", got.seq, got.volume.TotalVolume)
	}
}

// A session deleted while its report was being sent is not brought back when the
// report's outcome comes in.
func TestASettlementAfterTheDeletionDoesNotBringTheSessionBack(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	b := <-rs.monitor.batches // being sent

	if _, err := rs.pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, rs.localSEID, 2, 0)); err != nil {
		t.Fatalf("deletion refused: %v", err)
	}

	rs.monitor.settle(b, reportDelivered)

	if left := rs.pConn.store.GetAllSessions(); len(left) != 0 {
		t.Fatalf("the settlement put back %d sessions after the deletion", len(left))
	}
}

// Once the association is shutting down its sessions are being removed, and a report's
// outcome changes none of them.
func TestASettlementDuringShutdownChangesNoSession(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	b := <-rs.monitor.batches

	rs.pConn.isShutdown.Store(true)
	rs.monitor.settle(b, reportDelivered)

	if us := storedSession(t, rs.pConn, rs.localSEID).urrs[0].usage; us.outstanding == nil || !us.inFlight {
		t.Fatalf("a settlement during shutdown changed the session: %+v", us)
	}
}

// A response without its mandatory Cause does not say the report was counted, so the
// report is sent again unchanged.
func TestAnAnswerWithoutACauseLeavesTheReportOutstanding(t *testing.T) {
	rs := newReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	srreq, _ := rs.receive(t)

	rsp := message.NewSessionReportResponse(0, 0, 0, srreq.SequenceNumber, 0)

	buf := make([]byte, rsp.MarshalLen())
	if err := rsp.MarshalTo(buf); err != nil {
		t.Fatal(err)
	}

	rs.pConn.HandlePFCPMsg(buf)

	if us := rs.settled(t); us.outstanding == nil || us.outstanding.seq != 0 {
		t.Fatalf("the report answered without a Cause is not outstanding: %+v", us.outstanding)
	}

	rs.monitor.poll(time.Now())

	again, _ := rs.receive(t)
	if got := readUsageReport(t, again.UsageReport[0]); got.seq != 0 || got.volume.TotalVolume != 1500 {
		t.Fatalf("expected UR-SEQN 0 with 1500 bytes again, got UR-SEQN %d with %d", got.seq, got.volume.TotalVolume)
	}
}

// A poll that waits for a modification to release the session lock credits its traffic
// at a time after the modification: a URR the modification turned to measure duration is
// not metered from before it.
func TestAPollThatWaitsForAModificationMetersFromAfterIt(t *testing.T) {
	rs := newIdleReportingSetup(t)
	rs.reader.set(downlinkCounts(rs.localSEID, 500)) // below the 1000-byte threshold

	rs.pConn.sessionsMu.Lock()

	polled := make(chan struct{})

	go func() {
		rs.monitor.poll(time.Now())
		close(polled)
	}()

	time.Sleep(50 * time.Millisecond) // the poll reads, then waits for the lock

	s, _ := rs.pConn.store.GetSession(rs.localSEID)
	s.urrs = slices.Clone(s.urrs)
	turnedOn := time.Now()

	if err := s.UpdateURR(urr{urrID: testURRID, measureVolume: true, measureDuration: true},
		urrIEs{measurementMethod: true}, turnedOn); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if err := rs.pConn.store.PutSession(s); err != nil {
		t.Fatal(err)
	}

	rs.pConn.sessionsMu.Unlock()
	<-polled

	rs.pConn.sessionsMu.Lock()
	s, _ = rs.pConn.store.GetSession(rs.localSEID)
	rs.pConn.sessionsMu.Unlock()

	if from := s.urrs[0].usage.meteredTo; from.Before(turnedOn) {
		t.Fatalf("time is metered from %v, %v before duration measurement was turned on",
			from, turnedOn.Sub(from))
	}
}

// A Session Report Request of the most reports it may carry, each as large as a usage
// report gets, fits in one Ethernet frame.
func TestAFullSessionReportRequestFitsInAFrame(t *testing.T) {
	const maxFrameUDPPayload = 1500 - 20 - 8 // IPv4 and UDP headers

	largest := usageReport{
		urrID:       ^uint32(0),
		seq:         ^uint32(0),
		trigger:     triggerPERIO | triggerVOLTH | triggerTIMTH,
		start:       testEpoch,
		end:         testEpoch.Add(time.Hour),
		hasVolume:   true,
		volume:      volumeCounts{total: ^uint64(0), uplink: ^uint64(0), downlink: ^uint64(0)},
		hasDuration: true,
		duration:    time.Duration(^uint32(0)) * time.Second,
	}

	srreq := message.NewSessionReportRequest(0, 0, ^uint64(0), 0xFFFFFF, 0, ie.NewReportType(0, 0, 1, 0))
	for range maxReportsPerRequest {
		srreq.UsageReport = append(srreq.UsageReport, largest.inSessionReportRequest())
	}

	if n := srreq.MarshalLen(); n > maxFrameUDPPayload {
		t.Fatalf("a request of %d reports is %d bytes, more than the %d of a frame",
			maxReportsPerRequest, n, maxFrameUDPPayload)
	}
}

// A session with more reports due than one request carries sends them in several.
func TestASessionsReportsBeyondOneRequestGoInAnother(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.pConn.sessionsMu.Lock()
	s, _ := rs.pConn.store.GetSession(rs.localSEID)

	s.urrs = slices.Clone(s.urrs)
	s.pdrs = slices.Clone(s.pdrs)

	for id := uint32(testURRID + 1); id <= testURRID+maxReportsPerRequest; id++ {
		u := s.urrs[0]
		u.urrID = id
		s.CreateURR(u, time.Now())
		s.pdrs[0].urrIDList = append(slices.Clone(s.pdrs[0].urrIDList), id)
	}

	if err := rs.pConn.store.PutSession(s); err != nil {
		t.Fatal(err)
	}

	rs.pConn.sessionsMu.Unlock()

	rs.reader.set(downlinkCounts(rs.localSEID, 1500)) // past every URR's threshold
	rs.monitor.poll(time.Now())

	var sizes []int

	for len(rs.monitor.batches) > 0 {
		sizes = append(sizes, len((<-rs.monitor.batches).reports))
	}

	if !slices.Equal(sizes, []int{maxReportsPerRequest, 1}) {
		t.Fatalf("the session's %d reports were sent in requests of %v, expected %d and 1",
			maxReportsPerRequest+1, sizes, maxReportsPerRequest)
	}
}

// A request is addressed to the session's CP SEID as it is sent, not as its reports were
// queued: a Session Modification may have changed the CP F-SEID meanwhile.
func TestAReportGoesToTheCPSEIDAModificationSetAfterItWasQueued(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	b := <-rs.monitor.batches // queued for a sender

	const movedSEID = testRemoteSEID + 1

	if _, err := rs.pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, rs.localSEID, 2, 0,
			ie.NewFSEID(movedSEID, net.ParseIP("127.0.0.1"), nil))); err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	go func() { rs.monitor.settle(b, rs.monitor.send(b)) }()

	srreq, _ := rs.receive(t)
	if srreq.SEID() != movedSEID {
		t.Fatalf("the request is addressed to SEID %#x, expected the modified %#x", srreq.SEID(), uint64(movedSEID))
	}

	rs.answer(t, srreq, ie.CauseRequestAccepted)
	rs.settled(t)
}

// The reports of a session deleted before they were sent are not sent: the control plane
// has no session to take them.
func TestTheReportsOfASessionDeletedBeforeTheyAreSentAreNotSent(t *testing.T) {
	rs := newIdleReportingSetup(t)

	rs.reader.set(downlinkCounts(rs.localSEID, 1500))
	rs.monitor.poll(time.Now())

	b := <-rs.monitor.batches // queued for a sender

	if _, err := rs.pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, rs.localSEID, 2, 0)); err != nil {
		t.Fatalf("deletion refused: %v", err)
	}

	if outcome := rs.monitor.send(b); outcome != reportUndelivered {
		t.Fatalf("the reports of the deleted session settled as %v, expected undelivered", outcome)
	}

	if err := rs.peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	if n, _, err := rs.peer.ReadFromUDP(make([]byte, 4096)); err == nil {
		t.Fatalf("a %d-byte request was sent for the deleted session", n)
	}
}
