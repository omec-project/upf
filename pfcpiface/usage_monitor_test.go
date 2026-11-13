// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omec-project/upf-epc/logger"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

// With usage reporting on, the URR poller is the only reader of the flow-measure counters:
// a read clears them, so a scrape that read them would take bytes from the usage the
// poller bills. Scrapes are served from the poller's last completed window instead.

// countingStub is a flowMeasureStub that counts every command sent to the datapath.
type countingStub struct {
	flowMeasureStub
	commands *atomic.Int32
}

func (s countingStub) ModuleCommand(
	ctx context.Context, req *pb.CommandRequest, opts ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	s.commands.Add(1)

	return s.flowMeasureStub.ModuleCommand(ctx, req, opts...)
}

// monitoredNode is a session-stats node whose UPF reports usage, polled by the monitor it
// returns, and a count of the commands its datapath has been sent.
func monitoredNode(t *testing.T, interval time.Duration) (*PFCPNode, *usageMonitor, *atomic.Int32) {
	t.Helper()

	commands := &atomic.Int32{}
	node := newSessionStatsNode(t, flowMeasureStub{})
	node.ctx = context.Background()
	node.upf.datapath = &bess{client: countingStub{commands: commands}}
	node.upf.enableUsageReporting = true

	reader, ok := node.upf.datapath.(usageReader)
	if !ok {
		t.Fatal("the bess datapath does not read usage counters")
	}

	node.usage = newUsageMonitor(node, reader, interval)

	return node, node.usage, commands
}

func TestAScrapeServesTheLastPollWithoutReadingTheDatapath(t *testing.T) {
	node, m, commands := monitoredNode(t, time.Minute)
	collector := NewPFCPNodeCollector(node)

	if n := testutil.CollectAndCount(collector); n != 0 {
		t.Fatalf("a scrape before the first poll served %d series, expected none", n)
	}

	m.poll(time.Now())

	afterPoll := commands.Load()
	if afterPoll != 4 {
		t.Fatalf("the poll sent %d commands, expected a flip and three reads", afterPoll)
	}

	ul := fmt.Sprintf(`direction="uplink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestULPDR, statsTestUEIP)
	dl := fmt.Sprintf(`direction="downlink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestDLPDR, statsTestUEIP)
	want := `
# HELP upf_session_tx_bytes Shows the total number of bytes for a given session in UPF. Downlink bytes include GTP-U encapsulation
# TYPE upf_session_tx_bytes gauge
upf_session_tx_bytes{` + ul + `} 11000
upf_session_tx_bytes{` + dl + `} 8400
`

	for range 2 {
		err := testutil.CollectAndCompare(collector, strings.NewReader(want), "upf_session_tx_bytes")
		if err != nil {
			t.Fatalf("the scrape did not serve the poll's counts:\n%v", err)
		}
	}

	if n := commands.Load(); n != afterPoll {
		t.Fatalf("the scrapes sent %d commands to the datapath, expected none", n-afterPoll)
	}
}

func TestAStalledPollerServesNoSeries(t *testing.T) {
	const interval = time.Minute

	node, m, _ := monitoredNode(t, interval)

	m.poll(time.Now().Add(-2*interval - time.Second))

	if n := testutil.CollectAndCount(NewPFCPNodeCollector(node)); n != 0 {
		t.Fatalf("a snapshot older than two poll intervals served %d series, expected none", n)
	}
}

// Every read adds into the poll's window, so a read between two polls does not take its
// counts out of the next snapshot.
func TestAReadBetweenPollsStaysInTheNextSnapshot(t *testing.T) {
	node, m, _ := monitoredNode(t, time.Minute)

	m.readMu.Lock()
	m.read()
	m.readMu.Unlock()

	m.poll(time.Now())

	ul := fmt.Sprintf(`direction="uplink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestULPDR, statsTestUEIP)
	dl := fmt.Sprintf(`direction="downlink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestDLPDR, statsTestUEIP)
	want := `
# HELP upf_session_tx_bytes Shows the total number of bytes for a given session in UPF. Downlink bytes include GTP-U encapsulation
# TYPE upf_session_tx_bytes gauge
upf_session_tx_bytes{` + ul + `} 22000
upf_session_tx_bytes{` + dl + `} 16800
`

	err := testutil.CollectAndCompare(NewPFCPNodeCollector(node), strings.NewReader(want), "upf_session_tx_bytes")
	if err != nil {
		t.Fatalf("the snapshot does not hold both reads of its window:\n%v", err)
	}
}

func TestMergeFlowMeasureTablesAddsCountsAndKeepsTheBusierReadsPercentiles(t *testing.T) {
	key := flowMeasureKey{statsTestFSEID, statsTestULPDR}

	quiet := statsTestStatistic(statsTestULPDR, 2, 200)
	busy := statsTestStatistic(statsTestULPDR, 10, 1000)
	busy.Latency = &pb.FlowMeasureReadResponse_Statistic_Histogram{PercentileValuesNs: []uint64{1, 2, 3}}

	merged := mergeFlowMeasureTables(flowMeasureTable{key: quiet}, flowMeasureTable{key: busy}, map[flowMeasureKey]uint64{})[key]

	if merged.TotalPackets != 12 || merged.TotalBytes != 1200 {
		t.Fatalf("merged to %d packets and %d bytes, expected 12 and 1200", merged.TotalPackets, merged.TotalBytes)
	}

	if merged.Latency != busy.Latency {
		t.Fatal("the merged row does not keep the percentiles of the read with more packets")
	}

	if mergeFlowMeasureTables(nil, nil, map[flowMeasureKey]uint64{}) != nil {
		t.Fatal("two failed reads merged into a table that reads as read")
	}

	other := flowMeasureKey{statsTestFSEID, statsTestULPDR + 1}

	if got := mergeFlowMeasureTables(nil, flowMeasureTable{key: quiet}, map[flowMeasureKey]uint64{}); got[key] != quiet {
		t.Fatal("a read merged after a failed one was not kept")
	}

	if got := mergeFlowMeasureTables(flowMeasureTable{key: busy}, flowMeasureTable{other: quiet}, map[flowMeasureKey]uint64{}); got[other] != quiet {
		t.Fatal("a row only the second read saw was not kept")
	}
}

// Over several reads, a row keeps the percentiles of the read that saw the most of its
// packets, not of a read that saw more than the reads before it together.
func TestMergeFlowMeasureTablesKeepsTheBusiestOfSeveralReadsPercentiles(t *testing.T) {
	key := flowMeasureKey{statsTestFSEID, statsTestULPDR}

	read := func(packets, latency uint64) flowMeasureTable {
		s := statsTestStatistic(statsTestULPDR, packets, packets*100)
		s.Latency = &pb.FlowMeasureReadResponse_Statistic_Histogram{PercentileValuesNs: []uint64{latency}}

		return flowMeasureTable{key: s}
	}

	for _, tc := range []struct {
		name    string
		packets [3]uint64
		latency uint64
	}{
		{"the last read is the busiest", [3]uint64{60, 40, 70}, 2},
		{"the first read is the busiest", [3]uint64{60, 40, 50}, 0},
		{"the second read is the busiest", [3]uint64{40, 60, 50}, 1},
	} {
		picked := map[flowMeasureKey]uint64{}

		var window flowMeasureTable
		for i, packets := range tc.packets {
			window = mergeFlowMeasureTables(window, read(packets, uint64(i)), picked)
		}

		if got := window[key].Latency.PercentileValuesNs[0]; got != tc.latency {
			t.Errorf("%s: kept the percentiles of read %d, expected read %d", tc.name, got, tc.latency)
		}
	}
}

// signallingReader tells of each read made of it.
type signallingReader struct{ reads chan struct{} }

func (r signallingReader) ReadUsage(context.Context) (flowMeasureCounts, error) {
	select {
	case r.reads <- struct{}{}:
	default:
	}

	return flowMeasureCounts{preQos: flowMeasureTable{}, postUL: flowMeasureTable{}, postDL: flowMeasureTable{}}, nil
}

// The poller reads the datapath every interval until the node stops.
func TestThePollerReadsEachIntervalUntilTheNodeStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := signallingReader{reads: make(chan struct{})}
	m := newUsageMonitor(&PFCPNode{ctx: ctx}, reader, 10*time.Millisecond)

	stopped := make(chan struct{})

	go func() {
		m.run()
		close(stopped)
	}()

	for i := range 2 {
		select {
		case <-reader.reads:
		case <-time.After(5 * time.Second):
			t.Fatalf("the poller made %d reads, expected one each interval", i)
		}
	}

	cancel()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the poller did not stop with the node")
	}
}

func TestTheNodeReadsUsageOnlyWhenItCan(t *testing.T) {
	off := &PFCPNode{upf: &upf{datapath: &bess{}}}
	if usageMonitorFor(off) != nil {
		t.Fatal("a usage monitor was created with usage reporting off")
	}

	on := &PFCPNode{upf: &upf{datapath: &bess{}, enableUsageReporting: true, urrPollInterval: time.Second}}
	if m := usageMonitorFor(on); m == nil || m.interval != time.Second {
		t.Fatal("no usage monitor polling every urr_poll_interval was created for a datapath that reads usage")
	}

	// URRs accepted on a datapath that cannot read their usage would never be reported.
	cannot := &PFCPNode{upf: &upf{datapath: &fakeDP{}, enableUsageReporting: true, urrPollInterval: time.Second}}
	if usageMonitorFor(cannot) != nil || cannot.upf.enableUsageReporting {
		t.Fatal("usage reporting stayed on for a datapath that cannot read the usage counters")
	}
}

// Each poll gets and puts every session with URRs under the association's session lock,
// so the store must not format a session for a debug line that is not logged.
func TestTheStoreFormatsNoSessionForADebugLineThatIsOff(t *testing.T) {
	if logger.PfcpLog.Desugar().Core().Enabled(zapcore.DebugLevel) {
		t.Skip("debug logging is on")
	}

	s := PFCPSession{localSEID: 1}
	for id := range uint32(8) {
		s.CreatePDR(pdr{pdrID: id + 1, urrIDList: []uint32{testURRID}, hasURRIDs: true})
	}

	s.CreateURR(volumeThresholdURR(1000), testEpoch)

	store := NewInMemoryStore()

	// Formatting the session takes some 200 allocations; boxing its log arguments a few.
	const most = 8

	for _, op := range []struct {
		name string
		run  func()
	}{
		{"PutSession", func() {
			if err := store.PutSession(s); err != nil {
				t.Error(err)
			}
		}},
		{"GetSession", func() { store.GetSession(s.localSEID) }},
		{"GetAllSessions", func() { store.GetAllSessions() }},
	} {
		if got := testing.AllocsPerRun(100, op.run); got > most {
			t.Errorf("%s allocates %v times with debug logging off, expected at most %d", op.name, got, most)
		}
	}
}
