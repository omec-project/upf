// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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

// upfCollector provides all UPF metrics.
type upfCollector struct {
	packets *prometheus.Desc
	bytes   *prometheus.Desc
	dropped *prometheus.Desc

	latency *prometheus.Desc
	jitter  *prometheus.Desc

	sessionLatency             *prometheus.Desc
	sessionDropRate            *prometheus.Desc
	sessionThroughputBps       *prometheus.Desc
	sessionThroughputPps       *prometheus.Desc
	sessionObservationDuration *prometheus.Desc

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
		sessionLatency: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "latency_ns"),
			"Shows todo in UPF",
			[]string{"fseid", "pdr"}, nil,
		),
		sessionDropRate: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "drop_rate"),
			"Shows todo in UPF",
			[]string{"fseid", "pdr"}, nil,
		),
		sessionThroughputBps: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "throughput_bps"),
			"Shows todo in UPF",
			[]string{"fseid", "pdr"}, nil,
		),
		sessionThroughputPps: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "throughput_pps"),
			"Shows todo in UPF",
			[]string{"fseid", "pdr"}, nil,
		),
		sessionObservationDuration: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "observation_duration_ns"),
			"Shows todo in UPF",
			[]string{"fseid", "pdr"}, nil,
		),
		upf: upf,
	}
}

// Describe writes all descriptors to the prometheus desc channel.
func (uc *upfCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.packets
	ch <- uc.bytes
	ch <- uc.dropped

	ch <- uc.latency
	ch <- uc.jitter
}

// Collect writes all metrics to prometheus metric channel.
func (uc *upfCollector) Collect(ch chan<- prometheus.Metric) {
	uc.summaryLatencyJitter(ch)
	uc.portStats(ch)
	if err := uc.sessionStats(ch); err != nil {
		log.Error(err)
	}
}

func (uc *upfCollector) portStats(ch chan<- prometheus.Metric) {
	// When operating in sim mode there are no BESS ports
	uc.upf.portStats(uc, ch)
}

func (uc *upfCollector) summaryLatencyJitter(ch chan<- prometheus.Metric) {
	uc.upf.summaryLatencyJitter(uc, ch)
}

func (uc *upfCollector) sessionStats(ch chan<- prometheus.Metric) error {
	return uc.upf.sessionStats(uc, ch)
}

func setupProm(upf *upf) {
	uc := newUpfCollector(upf)
	prometheus.MustRegister(uc)

	http.Handle("/metrics", promhttp.Handler())
}
