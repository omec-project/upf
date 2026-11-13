// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/omec-project/upf-epc/logger"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

const (
	// drainCoalesce is how recent a read a drain accepts as its own. A deletion storm
	// would otherwise read the whole datapath once per deletion, on the reader goroutine
	// of the association sending it. The traffic a session forwards in the meantime,
	// before its rules are removed, goes unreported.
	drainCoalesce = 200 * time.Millisecond

	// reportSenders is how many Session Report Requests are in flight at once. Each waits
	// for its response, through the retransmissions max_req_retries allows.
	reportSenders = 16
	// reportQueueSize is how many Session Report Requests can wait for a sender. A report
	// that finds the queue full is not lost: it stays outstanding and is sent at the next
	// poll.
	reportQueueSize = 1024
	// maxReportsPerRequest is how many usage reports go in one Session Report Request; a
	// session's further reports go in further requests. It keeps a request within one
	// Ethernet frame: a control plane may read PFCP into a buffer not much larger (the
	// omec SMF reads 2048 bytes), and a request it cannot read would be sent again at
	// every poll.
	maxReportsPerRequest = 16
)

// usageReader is a datapath that can read the per-session usage counters.
type usageReader interface {
	// ReadUsage reads and clears the counts since the previous read. On a partial
	// failure it returns what it did read alongside the error: those counts have been
	// cleared and exist nowhere else.
	ReadUsage(ctx context.Context) (flowMeasureCounts, error)
}

// usageMonitor reads the datapath's usage counters for URR usage reporting. A read
// clears the counters, so it is their only reader: the per-session metrics are served
// from its last poll rather than read from the datapath again.
type usageMonitor struct {
	node     *PFCPNode
	reader   usageReader
	interval time.Duration

	// readMu is held across a read of the datapath and the use of what it read, so that
	// a drain that finds a read under way waits for its counts to be applied.
	readMu sync.Mutex
	// window is the counts read since the last periodic poll, and lastRead when the last
	// read began. picked is, for each of the window's tables in turn, the packets of the
	// read each row merged from several reads keeps the percentiles of. Under readMu.
	window   flowMeasureCounts
	picked   [3]map[flowMeasureKey]uint64
	lastRead time.Time

	snapshotMu sync.Mutex
	// snapshot is the window the last periodic poll closed, and snapshotAt when.
	snapshot   flowMeasureCounts
	snapshotAt time.Time

	// batches are the reports waiting for a sender, a session's to a batch.
	batches chan reportBatch
}

// reportBatch is up to maxReportsPerRequest reports of one session, sent in one Session
// Report Request.
type reportBatch struct {
	pConn     *PFCPConn
	localSEID uint64
	reports   []*usageReport
}

func newUsageMonitor(node *PFCPNode, reader usageReader, interval time.Duration) *usageMonitor {
	return &usageMonitor{
		node:     node,
		reader:   reader,
		interval: interval,
		batches:  make(chan reportBatch, reportQueueSize),
	}
}

// usageMonitorFor is the node's usage monitor: nil with usage reporting off, and nil on a
// datapath that cannot read the usage counters, which turns usage reporting off.
func usageMonitorFor(node *PFCPNode) *usageMonitor {
	upf := node.upf
	if !upf.enableUsageReporting {
		return nil
	}

	reader, ok := upf.datapath.(usageReader)
	if !ok {
		// URRs are then ignored, as they are with usage reporting off: accepting them
		// would tell the control plane its usage is being measured.
		logger.PfcpLog.Errorln("usage reporting is enabled, but this datapath cannot " +
			"read usage counters; URRs will be ignored")

		upf.enableUsageReporting = false

		return nil
	}

	return newUsageMonitor(node, reader, upf.urrPollInterval)
}

// run polls every interval, and sends the reports the polls generate, until the node
// shuts down.
func (m *usageMonitor) run() {
	for range reportSenders {
		go m.sendReports()
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.poll(time.Now())
		case <-m.node.ctx.Done():
			return
		}
	}
}

// poll reads the counters and publishes the window they close as the metrics snapshot.
func (m *usageMonitor) poll(now time.Time) {
	m.readMu.Lock()
	defer m.readMu.Unlock()

	m.read()

	m.snapshotMu.Lock()
	m.snapshot, m.snapshotAt = m.window, now
	m.snapshotMu.Unlock()

	m.window, m.picked = flowMeasureCounts{}, [3]map[flowMeasureKey]uint64{}
}

// drain brings every URR up to date with what the datapath has counted, so that a URR
// leaving its session can report all of its usage. A read that began under drainCoalesce
// before the call will do; the drain waits for its counts to be applied.
func (m *usageMonitor) drain() {
	asked := time.Now()

	m.readMu.Lock()
	defer m.readMu.Unlock()

	if asked.Sub(m.lastRead) < drainCoalesce {
		return
	}

	m.read()
}

// read reads the counters into the window, and credits them. Under readMu.
func (m *usageMonitor) read() {
	started := time.Now()

	// Bounded below the interval, so that a slow read is given up before the next poll.
	ctx, cancel := context.WithTimeout(m.node.ctx, m.interval*4/5)
	defer cancel()

	counts, err := m.reader.ReadUsage(ctx)
	if err != nil {
		logger.PfcpLog.Warnln("usage counter read failed; using the modules that were read:", err)
	}

	// Only a read of both post-QoS modules stands in for a drain's own: a session's usage
	// in the module that was not read would otherwise be lost with the session.
	if counts.postUL != nil && counts.postDL != nil {
		m.lastRead = started
	}

	if m.picked[0] == nil {
		m.picked = [3]map[flowMeasureKey]uint64{{}, {}, {}}
	}

	m.window = flowMeasureCounts{
		preQos: mergeFlowMeasureTables(m.window.preQos, counts.preQos, m.picked[0]),
		postUL: mergeFlowMeasureTables(m.window.postUL, counts.postUL, m.picked[1]),
		postDL: mergeFlowMeasureTables(m.window.postDL, counts.postDL, m.picked[2]),
	}
	m.apply(counts)
}

// pdrUsage is the volume the datapath counted for one PDR.
type pdrUsage struct {
	pdrID  uint32
	volume volumeCounts
}

// usageBySession sorts a read's post-QoS counts by session: usage is measured after QoS
// enforcement (TS 29.244 clause 5.2.2.3.1). The flow-measure tables are keyed by the
// session's local F-SEID.
//
// The volume billed is the user's packets: "the data as delivered to and forwarded from
// the user" (TS 32.255 clause 5.1.2). The modules count whole frames, so each packet's
// link and tunnel headers are taken off.
func usageBySession(counts flowMeasureCounts) map[uint64][]pdrUsage {
	usage := map[uint64][]pdrUsage{}

	for key, st := range counts.postUL {
		bytes := userBytes(st, counts.framing.uplink)
		usage[key.fseid] = append(usage[key.fseid], pdrUsage{
			uint32(key.pdr), volumeCounts{total: bytes, uplink: bytes},
		})
	}

	for key, st := range counts.postDL {
		bytes := userBytes(st, counts.framing.downlink)
		usage[key.fseid] = append(usage[key.fseid], pdrUsage{
			uint32(key.pdr), volumeCounts{total: bytes, downlink: bytes},
		})
	}

	return usage
}

// userBytes is the bytes of a row's packets less overhead bytes each.
func userBytes(st *pb.FlowMeasureReadResponse_Statistic, overhead uint64) uint64 {
	framing := st.TotalPackets * overhead
	if framing > st.TotalBytes {
		return 0
	}

	return st.TotalBytes - framing
}

// apply credits a read's counts to the URRs of every association's sessions, and queues
// the reports that fall due. Under readMu.
func (m *usageMonitor) apply(counts flowMeasureCounts) {
	usage := usageBySession(counts)

	m.node.pConns.Range(func(_, value any) bool {
		if pConn, ok := value.(*PFCPConn); ok {
			m.applyToConn(pConn, usage)
		}

		return true
	})
}

func (m *usageMonitor) applyToConn(pConn *PFCPConn, usage map[uint64][]pdrUsage) {
	var batches []reportBatch

	pConn.sessionsMu.Lock()

	// Taken under the lock, as a modification takes its time, so that the poll's time is
	// after that of any modification before it: a URR the modification turned to measure
	// duration is not metered from before it.
	now := time.Now()

	// Teardown snapshots the store under sessionsMu and then removes the sessions without
	// it; a session written back after its snapshot would outlive the association.
	if !pConn.IsShutdown() {
		for _, s := range pConn.store.GetAllSessions() {
			if len(s.urrs) == 0 {
				continue
			}

			s.urrs = slices.Clone(s.urrs)

			for _, u := range usage[s.localSEID] {
				s.addUsage(u.pdrID, u.volume, now)
			}

			reports := s.dueReports(now)

			if err := pConn.store.PutSession(s); err != nil {
				logger.PfcpLog.Errorf("failed to put PFCP session to store: %v", err)
				continue
			}

			for len(reports) > 0 {
				n := min(len(reports), maxReportsPerRequest)
				batches = append(batches, reportBatch{pConn, s.localSEID, reports[:n:n]})
				reports = reports[n:]
			}
		}
	}

	pConn.sessionsMu.Unlock()

	for _, b := range batches {
		select {
		case m.batches <- b:
		default:
			logger.PfcpLog.Warnln("usage report queue full; the reports of session",
				b.localSEID, "are sent at the next poll")
			m.settle(b, reportUndelivered)
		}
	}
}

// sendReports sends queued reports until the node shuts down.
func (m *usageMonitor) sendReports() {
	for {
		select {
		case b := <-m.batches:
			m.settle(b, m.send(b))
		case <-m.node.ctx.Done():
			return
		}
	}
}

// send sends a batch of reports in a Session Report Request and waits for its response,
// retransmitting it unchanged as the association's request settings allow (TS 29.244
// clause 6.4).
func (m *usageMonitor) send(b reportBatch) reportOutcome {
	remoteSEID, ok := b.remoteSEID()
	if !ok {
		return reportUndelivered // the session is gone, and its settlement changes nothing
	}

	srreq := message.NewSessionReportRequest(0, 0, remoteSEID, b.pConn.getSeqNum(), 0,
		ie.NewReportType(0, 0, 1, 0)) // USAR

	for _, r := range b.reports {
		srreq.UsageReport = append(srreq.UsageReport, r.inSessionReportRequest())
	}

	reply, timeout := b.pConn.sendPFCPRequestMessage(newRequest(srreq))
	if timeout || reply == nil {
		logger.PfcpLog.Warnln("no response to the usage reports of session", b.localSEID,
			"; they are sent again at the next poll")

		return reportUndelivered
	}

	srres, ok := reply.(*message.SessionReportResponse)
	if !ok || srres.Cause == nil {
		return reportUndelivered
	}

	cause, err := srres.Cause.Cause()
	if err != nil {
		return reportUndelivered
	}

	if cause != ie.CauseRequestAccepted {
		logger.PfcpLog.Warnln("the usage reports of session", b.localSEID,
			"were refused with cause", cause, "; their usage goes in the next report")

		return reportRejected
	}

	return reportDelivered
}

// remoteSEID is the batch's session's CP SEID as the request is built, not as the batch
// was queued: a Session Modification may since have changed the CP F-SEID.
func (b reportBatch) remoteSEID() (uint64, bool) {
	b.pConn.sessionsMu.Lock()
	defer b.pConn.sessionsMu.Unlock()

	s, ok := b.pConn.store.GetSession(b.localSEID)

	return s.remoteSEID, ok
}

// settle records what became of a batch in its session.
func (m *usageMonitor) settle(b reportBatch, outcome reportOutcome) {
	b.pConn.sessionsMu.Lock()
	defer b.pConn.sessionsMu.Unlock()

	if b.pConn.IsShutdown() {
		return
	}

	s, ok := b.pConn.store.GetSession(b.localSEID)
	if !ok {
		return
	}

	s.urrs = slices.Clone(s.urrs)
	s.settleReports(b.reports, outcome)

	if err := b.pConn.store.PutSession(s); err != nil {
		logger.PfcpLog.Errorf("failed to put PFCP session to store: %v", err)
	}
}

// collectSessionStats sends the per-session series of the last poll's window. A snapshot
// older than two intervals means the poller has stalled, and is not served: its traffic
// is not current.
func (m *usageMonitor) collectSessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) {
	m.snapshotMu.Lock()
	snapshot, at := m.snapshot, m.snapshotAt
	m.snapshotMu.Unlock()

	if at.IsZero() || time.Since(at) > 2*m.interval {
		return
	}

	emitSessionStats(pc, ch, snapshot)
}

// mergeFlowMeasureTables adds read b of one module into a, the reads before it. Bytes and
// packets add; latency and jitter percentiles cannot, so a row keeps those of the read
// that saw the most of its packets. picked holds that read's packets for each row merged
// from several reads; a row from one read has its own. Either table may be nil, meaning
// that read failed; the result is nil only if both did.
func mergeFlowMeasureTables(a, b flowMeasureTable, picked map[flowMeasureKey]uint64) flowMeasureTable {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}

	for key, s := range b {
		held, ok := a[key]
		if !ok {
			a[key] = s
			continue
		}

		merged := &pb.FlowMeasureReadResponse_Statistic{
			Fseid:        held.Fseid,
			Pdr:          held.Pdr,
			Latency:      held.Latency,
			Jitter:       held.Jitter,
			TotalBytes:   held.TotalBytes + s.TotalBytes,
			TotalPackets: held.TotalPackets + s.TotalPackets,
		}

		most, ok := picked[key]
		if !ok {
			most = held.TotalPackets
		}

		if s.TotalPackets > most {
			merged.Latency, merged.Jitter, most = s.Latency, s.Jitter, s.TotalPackets
		}

		picked[key] = most

		a[key] = merged
	}

	return a
}

// drainUsage brings the URRs of the association's sessions up to date with what the
// datapath has counted, when usage is reported. A session handler calls it before taking
// sessionsMu, which the drain takes to credit the counts.
func (pConn *PFCPConn) drainUsage() {
	if pConn.node != nil && pConn.node.usage != nil {
		pConn.node.usage.drain()
	}
}
