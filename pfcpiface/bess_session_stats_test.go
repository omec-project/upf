// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// The per-session metrics are the only per-subscriber throughput the UPF exports, so a
// reader has to be able to tell a UE's uplink from its downlink. The collector reads the
// two directions from separate post-QoS flow-measure modules; every series must say which
// one it came from, without the reader having to know how the SMF numbers its rules.

const (
	statsTestFSEID = 7
	statsTestUEIP  = "10.250.0.1"
	// The downlink rule deliberately has the lower ID. The SMF usually creates the uplink
	// rule first, so a direction guessed from rule order would get this session backwards.
	statsTestDLPDR = 10
	statsTestULPDR = 20
)

// flowMeasureStub answers the flip and read commands SessionStats sends to the three
// flow-measure modules, returning one uplink and one downlink rule for a single session.
type flowMeasureStub struct {
	pb.BESSControlClient
}

func statsTestStatistic(pdrID, packets, bytes uint64) *pb.FlowMeasureReadResponse_Statistic {
	return &pb.FlowMeasureReadResponse_Statistic{
		Fseid:        statsTestFSEID,
		Pdr:          pdrID,
		TotalPackets: packets,
		TotalBytes:   bytes,
		Latency: &pb.FlowMeasureReadResponse_Statistic_Histogram{
			PercentileValuesNs: []uint64{100, 200, 300},
		},
		Jitter: &pb.FlowMeasureReadResponse_Statistic_Histogram{
			PercentileValuesNs: []uint64{10, 20, 30},
		},
	}
}

func (flowMeasureStub) ModuleCommand(
	_ context.Context, req *pb.CommandRequest, _ ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	var stats []*pb.FlowMeasureReadResponse_Statistic

	switch req.Name {
	case PreQosFlowMeasure:
		stats = []*pb.FlowMeasureReadResponse_Statistic{
			statsTestStatistic(statsTestULPDR, 12, 12600),
			statsTestStatistic(statsTestDLPDR, 9, 9500),
		}
	case PostUlQosFlowMeasure:
		stats = []*pb.FlowMeasureReadResponse_Statistic{statsTestStatistic(statsTestULPDR, 11, 11000)}
	case PostDlQosFlowMeasure:
		stats = []*pb.FlowMeasureReadResponse_Statistic{statsTestStatistic(statsTestDLPDR, 8, 8400)}
	}

	var msg proto.Message

	switch {
	case req.Cmd == "flip":
		msg = &pb.FlowMeasureFlipResponse{OldFlag: 1}
	case req.Cmd == "read" && stats != nil:
		msg = &pb.FlowMeasureReadResponse{Statistics: stats}
	default:
		return nil, fmt.Errorf("unexpected module command %q on %q", req.Cmd, req.Name)
	}

	data, err := anypb.New(msg)
	if err != nil {
		return nil, err
	}

	return &pb.CommandResponse{Data: data}, nil
}

// newSessionStatsNode builds a node whose datapath answers with flowMeasureStub, and whose
// one association knows the session, so the UE address resolves from its uplink rule.
func newSessionStatsNode(t *testing.T) *PFCPNode {
	t.Helper()

	store := NewInMemoryStore()

	err := store.PutSession(PFCPSession{
		localSEID: statsTestFSEID,
		PacketForwardingRules: PacketForwardingRules{pdrs: []pdr{
			{srcIface: access, ueAddress: ip2int(net.ParseIP(statsTestUEIP))},
			{srcIface: core},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	node := &PFCPNode{upf: &upf{enableFlowMeasure: true, datapath: &bess{client: flowMeasureStub{}}}}
	node.pConns.Store("smf", &PFCPConn{store: store})

	return node
}

// Each series names the direction of the module it was read from, and nothing else about
// it changes: the other labels, the values and the number of series are what the stub
// supplied, one series per rule and metric.
func TestSessionStatsLabelsEachSeriesWithItsDirection(t *testing.T) {
	ul := fmt.Sprintf(`direction="uplink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestULPDR, statsTestUEIP)
	dl := fmt.Sprintf(`direction="downlink",fseid="%d",pdr="%d",ue_ip="%s"`, statsTestFSEID, statsTestDLPDR, statsTestUEIP)

	want := `
# HELP upf_session_tx_bytes Shows the total number of bytes for a given session in UPF. Downlink bytes include GTP-U encapsulation
# TYPE upf_session_tx_bytes gauge
upf_session_tx_bytes{` + ul + `} 11000
upf_session_tx_bytes{` + dl + `} 8400
# HELP upf_session_tx_packets Shows the total number of packets sent for a given session in UPF
# TYPE upf_session_tx_packets gauge
upf_session_tx_packets{` + ul + `} 11
upf_session_tx_packets{` + dl + `} 8
# HELP upf_session_rx_packets Shows the total number of packets received for a given session in UPF
# TYPE upf_session_rx_packets gauge
upf_session_rx_packets{` + ul + `} 12
upf_session_rx_packets{` + dl + `} 9
# HELP upf_session_latency_ns Shows the latency of a session in UPF
# TYPE upf_session_latency_ns summary
upf_session_latency_ns{` + ul + `,quantile="50"} 100
upf_session_latency_ns{` + ul + `,quantile="90"} 200
upf_session_latency_ns{` + ul + `,quantile="99"} 300
upf_session_latency_ns_sum{` + ul + `} 0
upf_session_latency_ns_count{` + ul + `} 11
upf_session_latency_ns{` + dl + `,quantile="50"} 100
upf_session_latency_ns{` + dl + `,quantile="90"} 200
upf_session_latency_ns{` + dl + `,quantile="99"} 300
upf_session_latency_ns_sum{` + dl + `} 0
upf_session_latency_ns_count{` + dl + `} 8
# HELP upf_session_jitter_ns Shows the jitter of a session in UPF
# TYPE upf_session_jitter_ns summary
upf_session_jitter_ns{` + ul + `,quantile="50"} 10
upf_session_jitter_ns{` + ul + `,quantile="90"} 20
upf_session_jitter_ns{` + ul + `,quantile="99"} 30
upf_session_jitter_ns_sum{` + ul + `} 0
upf_session_jitter_ns_count{` + ul + `} 11
upf_session_jitter_ns{` + dl + `,quantile="50"} 10
upf_session_jitter_ns{` + dl + `,quantile="90"} 20
upf_session_jitter_ns{` + dl + `,quantile="99"} 30
upf_session_jitter_ns_sum{` + dl + `} 0
upf_session_jitter_ns_count{` + dl + `} 8
`

	collector := NewPFCPNodeCollector(newSessionStatsNode(t))

	if err := testutil.CollectAndCompare(collector, strings.NewReader(want)); err != nil {
		t.Errorf("session metrics do not name the direction each series was read from "+
			"(the downlink rule has the lower PDR ID, so rule order must not decide it):\n%v", err)
	}
}

// A descriptor that is declared but not yet emitted produces no series for the test above
// to inspect, so check the declarations themselves: whoever starts emitting dropped
// packets must not be able to produce a series without a direction.
func TestSessionMetricDescriptorsAllDeclareDirection(t *testing.T) {
	c := NewPFCPNodeCollector(&PFCPNode{})

	descs := map[string]*prometheus.Desc{
		"latency_ns":      c.sessionLatency,
		"jitter_ns":       c.sessionJitter,
		"tx_packets":      c.sessionTxPackets,
		"rx_packets":      c.sessionRxPackets,
		"dropped_packets": c.sessionDroppedPackets,
		"tx_bytes":        c.sessionTxBytes,
	}

	for name, d := range descs {
		// Bind the descriptor the way SessionStats does, and read back the label names it
		// was declared with.
		m, err := prometheus.NewConstMetric(d, prometheus.GaugeValue, 0, "fseid", "pdr", "ue_ip", "direction")
		if err != nil {
			t.Errorf("upf_session_%s does not take the %q label: %v", name, labelDirection, err)
			continue
		}

		var out dto.Metric
		if err := m.Write(&out); err != nil {
			t.Fatal(err)
		}

		if !slices.ContainsFunc(out.GetLabel(), func(l *dto.LabelPair) bool { return l.GetName() == labelDirection }) {
			t.Errorf("upf_session_%s does not declare the %q label: %v", name, labelDirection, out.GetLabel())
		}
	}
}
