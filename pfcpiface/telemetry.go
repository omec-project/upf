// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"net/http"

	log "github.com/sirupsen/logrus"

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

// upfCollector provides all UPF metrics.
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
}

func (uc *upfCollector) portStats(ch chan<- prometheus.Metric) {
	// When operating in sim mode there are no BESS ports
	uc.upf.PortStats(uc, ch)
}

func (uc *upfCollector) summaryLatencyJitter(ch chan<- prometheus.Metric) {
	uc.upf.SummaryLatencyJitter(uc, ch)
}

// PfcpNodeCollector makes a PFCPNode Prometheus observable.
type PfcpNodeCollector struct {
	node                  *PFCPNode
	sessionLatency        *prometheus.Desc
	sessionJitter         *prometheus.Desc
	sessionTxPackets      *prometheus.Desc
	sessionRxPackets      *prometheus.Desc
	sessionDroppedPackets *prometheus.Desc
	sessionTxBytes        *prometheus.Desc
}

func NewPFCPNodeCollector(node *PFCPNode) *PfcpNodeCollector {
	return &PfcpNodeCollector{
		node: node,
		sessionLatency: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "latency_ns"),
			"Shows the latency of a session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
		sessionJitter: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "jitter_ns"),
			"Shows the jitter of a session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
		sessionTxPackets: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "tx_packets"),
			"Shows the total number of packets sent for a given session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
		sessionRxPackets: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "rx_packets"),
			"Shows the total number of packets received for a given session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
		sessionDroppedPackets: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "dropped_packets"),
			"Shows the number of packets dropped for a given session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
		sessionTxBytes: prometheus.NewDesc(prometheus.BuildFQName("upf", "session", "tx_bytes"),
			"Shows the total number of bytes for a given session in UPF",
			[]string{"fseid", "pdr", "ue_ip"}, nil,
		),
	}
}

func (col PfcpNodeCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(col, ch)
}

func (col PfcpNodeCollector) Collect(ch chan<- prometheus.Metric) {
	if col.node.upf.enableFlowMeasure {
		err := col.node.upf.SessionStats(&col, ch)
		if err != nil {
			log.Errorln(err)
			return
		}
	}
}

func setupProm(mux *http.ServeMux, upf *upf, node *PFCPNode) (*upfCollector, *PfcpNodeCollector, error) {
	uc := newUpfCollector(upf)
	if err := prometheus.Register(uc); err != nil {
		return nil, nil, err
	}

	nc := NewPFCPNodeCollector(node)
	if err := prometheus.Register(nc); err != nil {
		return nil, nil, err
	}

	mux.Handle("/metrics", promhttp.Handler())

	return uc, nc, nil
}

func clearProm(uc *upfCollector, nc *PfcpNodeCollector) {
	if ok := prometheus.Unregister(uc); !ok {
		log.Warnln("Failed to unregister upfCollector")
	}

	if ok := prometheus.Unregister(nc); !ok {
		log.Warnln("Failed to unregister PfcpNodeCollector")
	}
}
