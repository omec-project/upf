// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// interleaveStub answers like flowMeasureStub and records whether one collection's flip
// arrived while another collection was still between its own flip and its last read.
// A collection flips the leader, then reads the side the flip retired from all three
// modules; a second flip in between makes the first collection read a side that traffic
// is writing again.
type interleaveStub struct {
	flowMeasureStub

	mu          sync.Mutex
	open        bool // a collection has flipped and not yet made its third read
	reads       int
	interleaved bool
}

func (s *interleaveStub) ModuleCommand(
	ctx context.Context, req *pb.CommandRequest, opts ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	isFlip := req.GetCmd() == "flip"

	s.mu.Lock()

	if isFlip {
		if s.open {
			s.interleaved = true
		}

		s.open = true
		s.reads = 0
	} else {
		s.reads++
		if s.reads == 3 {
			s.open = false
		}
	}

	s.mu.Unlock()

	if isFlip {
		// bessd's flip waits 10 ms for packets carrying the old flag to drain, which is
		// the window an overlapping collection lands in.
		time.Sleep(10 * time.Millisecond)
	}

	return s.flowMeasureStub.ModuleCommand(ctx, req, opts...)
}

// Prometheus runs a scrape per request, so two scrapes of the UPF can collect at once.
// Each collection flips the leader and then reads the retired side; the flips and reads of
// two collections must not interleave.
func TestSessionStatsCollectionsDoNotInterleave(t *testing.T) {
	stub := &interleaveStub{}
	node := &PFCPNode{upf: &upf{enableFlowMeasure: true, datapath: &bess{client: stub}}}
	col := NewPFCPNodeCollector(node)

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			ch := make(chan prometheus.Metric, 64)
			if err := node.upf.SessionStats(col, ch); err != nil {
				t.Errorf("SessionStats: %v", err)
			}
		})
	}

	wg.Wait()

	if stub.interleaved {
		t.Error("a collection flipped the leader while another was still reading the side its flip retired")
	}
}
