// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"net/http"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func getPctiles() []float64 {
	return []float64{50, 75, 90, 95, 99, 99.9, 99.99, 99.999, 99.9999, 100}
}

func makeBuckets(values []uint64) map[float64]float64 {
	buckets := make(map[float64]float64)
	for idx, pctile := range getPctiles() {
		buckets[pctile] = float64(values[idx])
	}
	return buckets
}

//upfCollector provides all UPF metrics
type upfCollector struct {
	packets *prometheus.Desc
	bytes   *prometheus.Desc
	dropped *prometheus.Desc

	latency *prometheus.Desc
	jitter  *prometheus.Desc

	upf *upf
}

func newUpfCollector(upf *upf) *upfCollector {
	return &upfCollector{
		packets: prometheus.NewDesc(prometheus.BuildFQName("upf", "packets", "count"),
			"Shows the number of packets received by the UPF port",
			[]string{"iface", "dir"}, nil,
		),
		bytes: prometheus.NewDesc(prometheus.BuildFQName("upf", "bytes", "count"),
			"Shows the number of bytes received by the UPF port",
			[]string{"iface", "dir"}, nil,
		),
		dropped: prometheus.NewDesc(prometheus.BuildFQName("upf", "dropped", "count"),
			"Shows the number of packets dropped on receive by the UPF port",
			[]string{"iface", "dir"}, nil,
		),
		latency: prometheus.NewDesc(prometheus.BuildFQName("upf", "latency", "ns"),
			"Shows the packet processing latency percentiles in UPF",
			[]string{"iface"}, nil,
		),
		jitter: prometheus.NewDesc(prometheus.BuildFQName("upf", "jitter", "ns"),
			"Shows the packet processing jitter percentiles in UPF",
			[]string{"iface"}, nil,
		),
		upf: upf,
	}
}

//Describe writes all descriptors to the prometheus desc channel.
func (uc *upfCollector) Describe(ch chan<- *prometheus.Desc) {

	ch <- uc.packets
	ch <- uc.bytes
	ch <- uc.dropped

	ch <- uc.latency
	ch <- uc.jitter
}

//Collect writes all metrics to prometheus metric channel
func (uc *upfCollector) Collect(ch chan<- prometheus.Metric) {
	uc.summaryLatencyJitter(ch)
	uc.portStats(ch)
}

func (uc *upfCollector) portStats(ch chan<- prometheus.Metric) {
	// When operating in sim mode there are no BESS ports
	if uc.upf.simInfo != nil {
		return
	}

	portstats := func(ifaceLabel, ifaceName string) {
		packets := func(packets uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.packets,
				prometheus.CounterValue,
				float64(packets),
				ifaceLabel, direction,
			)
			ch <- p
		}
		bytes := func(bytes uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.bytes,
				prometheus.CounterValue,
				float64(bytes),
				ifaceLabel, direction,
			)
			ch <- p
		}
		dropped := func(dropped uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.dropped,
				prometheus.CounterValue,
				float64(dropped),
				ifaceLabel, direction,
			)
			ch <- p
		}

		res := uc.upf.portStats(ifaceName)
		if res == nil {
			return
		}

		packets(res.Inc.Packets, "rx")
		packets(res.Out.Packets, "tx")

		bytes(res.Inc.Bytes, "rx")
		bytes(res.Out.Bytes, "tx")

		dropped(res.Inc.Dropped, "rx")
		dropped(res.Out.Dropped, "tx")

	}

	portstats("Access", uc.upf.accessIface)
	portstats("Core", uc.upf.coreIface)
}

func (uc *upfCollector) summaryLatencyJitter(ch chan<- prometheus.Metric) {
	measureIface := func(ifaceLabel, ifaceName string) {
		req := &pb.MeasureCommandGetSummaryArg{
			Clear:              true,
			LatencyPercentiles: getPctiles(),
			JitterPercentiles:  getPctiles(),
		}
		res := uc.upf.measure(ifaceName, req)
		if res == nil {
			return
		}

		latencies := res.GetLatency().GetPercentileValuesNs()
		if latencies != nil {
			l := prometheus.MustNewConstSummary(
				uc.latency,
				res.Packets,
				float64(res.Latency.GetTotalNs()),
				makeBuckets(latencies),
				ifaceLabel,
			)

			ch <- l
		}

		jitters := res.GetJitter().GetPercentileValuesNs()
		if jitters != nil {
			j := prometheus.MustNewConstSummary(
				uc.jitter,
				res.Packets,
				float64(res.Jitter.GetTotalNs()),
				makeBuckets(jitters),
				ifaceLabel,
			)

			ch <- j
		}
	}
	measureIface("Access", uc.upf.accessIface)
	measureIface("Core", uc.upf.coreIface)
}

func setupProm(upf *upf) {
	uc := newUpfCollector(upf)
	prometheus.MustRegister(uc)
	http.Handle("/metrics", promhttp.Handler())
}
