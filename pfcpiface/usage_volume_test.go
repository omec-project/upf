// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
)

// The volume billed is the user's packets: "the data as delivered to and forwarded from
// the user" (TS 32.255 clause 5.1.2). The post-QoS flow-measure modules count whole
// frames: the uplink module, after decapsulation, an Ethernet header on each packet; the
// downlink module, after encapsulation, the GTP-U tunnel's headers as well.

func TestBessReportsTheFramingItsModulesCount(t *testing.T) {
	for _, tc := range []struct {
		gtppsc bool
		want   frameOverhead
	}{
		{false, frameOverhead{uplink: 14, downlink: 14 + 20 + 8 + 8}},
		{true, frameOverhead{uplink: 14, downlink: 14 + 20 + 8 + 8 + 8}},
	} {
		counts, err := (&bess{client: flowMeasureStub{}, gtppsc: tc.gtppsc}).ReadUsage(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if counts.framing != tc.want {
			t.Errorf("gtppsc=%t: a read reports framing %+v, expected %+v", tc.gtppsc, counts.framing, tc.want)
		}
	}
}

func TestTheBilledVolumeLeavesOutEachPacketsFraming(t *testing.T) {
	key := flowMeasureKey{statsTestFSEID, downlinkPDRID}
	row := func(packets, bytes uint64) flowMeasureTable {
		return flowMeasureTable{key: &pb.FlowMeasureReadResponse_Statistic{
			Fseid: statsTestFSEID, Pdr: downlinkPDRID, TotalPackets: packets, TotalBytes: bytes,
		}}
	}

	usage := usageBySession(flowMeasureCounts{
		postUL:  row(10, 10*(1000+14)),
		postDL:  row(10, 10*(1000+50)),
		framing: frameOverhead{uplink: 14, downlink: 50},
	})[statsTestFSEID]

	if len(usage) != 2 {
		t.Fatalf("expected an uplink and a downlink count, got %+v", usage)
	}

	if usage[0].volume != uplink(10_000) || usage[1].volume != downlink(10_000) {
		t.Fatalf("billed %+v, expected 10000 bytes each way", usage)
	}

	short := usageBySession(flowMeasureCounts{
		postDL:  row(10, 100),
		framing: frameOverhead{downlink: 50},
	})[statsTestFSEID]

	if short[0].volume.total != 0 {
		t.Fatalf("a count smaller than its framing billed %d bytes, expected 0", short[0].volume.total)
	}
}
