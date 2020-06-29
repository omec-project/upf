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
	return []float64{25, 50, 75, 90, 99}
}

func makeBuckets(values []uint64) map[float64]uint64 {
	buckets := make(map[float64]uint64)
	for idx, pctile := range getPctiles() {
		buckets[pctile] = values[idx]
	}
	return buckets
}

//upfCollector provides all UPF metrics
type upfCollector struct {
	rxPackets *prometheus.Desc
	rxBytes   *prometheus.Desc
	rxDropped *prometheus.Desc

	txPackets *prometheus.Desc
	txBytes   *prometheus.Desc
	txDropped *prometheus.Desc

	latency *prometheus.Desc
	jitter  *prometheus.Desc

	upf *upf
}

func newUpfCollector(upf *upf) *upfCollector {
	return &upfCollector{
		rxPackets: prometheus.NewDesc(prometheus.BuildFQName("upf", "rx_packets", "count"),
			"Shows the number of packets received by the UPF port",
			[]string{"iface"}, nil,
		),
		rxBytes: prometheus.NewDesc(prometheus.BuildFQName("upf", "rx_bytes", "count"),
			"Shows the number of bytes received by the UPF port",
			[]string{"iface"}, nil,
		),
		rxDropped: prometheus.NewDesc(prometheus.BuildFQName("upf", "rx_dropped", "count"),
			"Shows the number of packets dropped on receive by the UPF port",
			[]string{"iface"}, nil,
		),
		txPackets: prometheus.NewDesc(prometheus.BuildFQName("upf", "tx_packets", "count"),
			"Shows the number of packets received by the UPF port",
			[]string{"iface"}, nil,
		),
		txBytes: prometheus.NewDesc(prometheus.BuildFQName("upf", "tx_bytes", "count"),
			"Shows the number of bytes received by the UPF port",
			[]string{"iface"}, nil,
		),
		txDropped: prometheus.NewDesc(prometheus.BuildFQName("upf", "tx_dropped", "count"),
			"Shows the number of packets dropped by the UPF port",
			[]string{"iface"}, nil,
		),
		latency: prometheus.NewDesc(prometheus.BuildFQName("upf", "latency", "hist"),
			"Shows the packet processing latency in UPF",
			[]string{"iface"}, nil,
		),
		jitter: prometheus.NewDesc(prometheus.BuildFQName("upf", "jitter", "hist"),
			"Shows the packet processing jitter in UPF",
			[]string{"iface"}, nil,
		),
		upf: upf,
	}
}

//Describe writes all descriptors to the prometheus desc channel.
func (uc *upfCollector) Describe(ch chan<- *prometheus.Desc) {

	ch <- uc.rxPackets
	ch <- uc.rxBytes
	ch <- uc.rxDropped

	ch <- uc.txPackets
	ch <- uc.txBytes
	ch <- uc.txDropped

	ch <- uc.latency
	ch <- uc.jitter
}

//Collect writes all metrics to prometheus metric channel
func (uc *upfCollector) Collect(ch chan<- prometheus.Metric) {
	uc.histLatencyJitter(ch)
	uc.portStats(ch)
}

func (uc *upfCollector) portStats(ch chan<- prometheus.Metric) {
}

func (uc *upfCollector) histLatencyJitter(ch chan<- prometheus.Metric) {
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
			l := prometheus.MustNewConstHistogram(
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
			j := prometheus.MustNewConstHistogram(
				uc.jitter,
				res.Packets,
				float64(res.Jitter.GetTotalNs()),
				makeBuckets(jitters),
				ifaceLabel,
			)

			ch <- j
		}
	}
	measureIface("N3", uc.upf.n3Iface)
	measureIface("N6", uc.upf.n6Iface)
}

func setupProm(upf *upf) {
	uc := newUpfCollector(upf)
	prometheus.MustRegister(uc)
	http.Handle("/metrics", promhttp.Handler())
}
