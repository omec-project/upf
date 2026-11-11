// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"strconv"

	"github.com/omec-project/upf-epc/logger"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
)

// sessionStatsQuantiles are the latency and jitter percentiles read from the flow-measure
// modules and reported in the per-session metrics.
var sessionStatsQuantiles = []float64{50, 90, 99}

// flowMeasureKey names one row of a flow-measure table: a PDR of a session.
type flowMeasureKey struct {
	fseid, pdr uint64
}

type flowMeasureTable map[flowMeasureKey]*pb.FlowMeasureReadResponse_Statistic

func newFlowMeasureTable(resp *pb.FlowMeasureReadResponse) flowMeasureTable {
	if resp == nil {
		return nil
	}

	t := make(flowMeasureTable, len(resp.Statistics))
	for _, s := range resp.Statistics {
		t[flowMeasureKey{s.Fseid, s.Pdr}] = s
	}

	return t
}

// flowMeasureCounts is one read of the three flow-measure modules. A table is nil when
// its module could not be read.
type flowMeasureCounts struct {
	preQos, postUL, postDL flowMeasureTable

	// framing is how many bytes of each packet the post-QoS modules count that are not
	// the user's: link and tunnel headers.
	framing frameOverhead
}

// frameOverhead is the bytes per packet, in each direction, that a flow-measure module
// counts on top of the user's packet.
type frameOverhead struct {
	uplink, downlink uint64
}

// emitSessionStats sends the per-session series of the counts. A direction needs both its
// post-QoS table and the pre-QoS one, which carries rx_packets.
func emitSessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric, counts flowMeasureCounts) {
	if counts.preQos == nil {
		return
	}

	// TODO: pick first connection for now
	var con *PFCPConn

	pc.node.pConns.Range(func(key, value any) bool {
		pConn, ok := value.(*PFCPConn)
		if !ok {
			return false
		}

		con = pConn

		return false
	})

	if con == nil {
		logger.BessLog.Warnln("no active PFCP connection, UE IP lookup disabled")
	}

	if counts.postUL != nil {
		emitDirection(pc, ch, con, counts.preQos, counts.postUL, "uplink")
	}

	if counts.postDL != nil {
		emitDirection(pc, ch, con, counts.preQos, counts.postDL, "downlink")
	}
}

func emitDirection(
	pc *PfcpNodeCollector, ch chan<- prometheus.Metric, con *PFCPConn,
	preQos, postQos flowMeasureTable, direction string,
) {
	q := sessionStatsQuantiles

	for key, post := range postQos {
		pre, ok := preQos[key]
		if !ok {
			logger.BessLog.Infof("found no pre QoS statistics for PDR %v FSEID %v", post.Pdr, post.Fseid)
			continue
		}

		fseidString := strconv.FormatUint(pre.Fseid, 10)
		pdrString := strconv.FormatUint(pre.Pdr, 10)

		ueIpString, ok := resolveUEIP(con, pre, UnknownString)
		if !ok {
			continue
		}

		ch <- prometheus.MustNewConstMetric(
			pc.sessionTxPackets,
			prometheus.GaugeValue,
			float64(post.TotalPackets),
			fseidString,
			pdrString,
			ueIpString,
			direction,
		)
		ch <- prometheus.MustNewConstMetric(
			pc.sessionRxPackets,
			prometheus.GaugeValue,
			float64(pre.TotalPackets),
			fseidString,
			pdrString,
			ueIpString,
			direction,
		)
		ch <- prometheus.MustNewConstMetric(
			pc.sessionTxBytes,
			prometheus.GaugeValue,
			float64(post.TotalBytes),
			fseidString,
			pdrString,
			ueIpString,
			direction,
		)
		ch <- prometheus.MustNewConstSummary(
			pc.sessionLatency,
			post.TotalPackets,
			0,
			map[float64]float64{
				q[0]: float64(post.Latency.PercentileValuesNs[0]),
				q[1]: float64(post.Latency.PercentileValuesNs[1]),
				q[2]: float64(post.Latency.PercentileValuesNs[2]),
			},
			fseidString,
			pdrString,
			ueIpString,
			direction,
		)
		ch <- prometheus.MustNewConstSummary(
			pc.sessionJitter,
			post.TotalPackets,
			0,
			map[float64]float64{
				q[0]: float64(post.Jitter.PercentileValuesNs[0]),
				q[1]: float64(post.Jitter.PercentileValuesNs[1]),
				q[2]: float64(post.Jitter.PercentileValuesNs[2]),
			},
			fseidString,
			pdrString,
			ueIpString,
			direction,
		)
	}
}
