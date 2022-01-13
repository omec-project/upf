package main

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"net"
	"strconv"

	reuse "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

// PFCPNode represents a PFCP endpoint of the UPF.
type PFCPNode struct {
	ctx context.Context
	// listening socket for new "PFCP connections"
	net.PacketConn
	// done is closed to signal shutdown complete
	done chan struct{}
	// channel for PFCPConn to signal exit by sending their remote address
	pConnDone chan string
	// map of existing connections
	pConns map[string]*PFCPConn
	// upf
	upf *upf
	// metrics for PFCP messages and sessions
	metrics metrics.InstrumentPFCP

	collector *PfcpNodeCollector
}

// NewPFCPNode create a new PFCPNode listening on local address.
func NewPFCPNode(ctx context.Context, upf *upf) *PFCPNode {
	conn, err := reuse.ListenPacket("udp", ":"+PFCPPort)
	if err != nil {
		log.Fatalln("ListenUDP failed", err)
	}

	metrics, err := metrics.NewPrometheusService()
	if err != nil {
		log.Fatalln("prom metrics service init failed", err)
	}

	node := &PFCPNode{
		ctx:        ctx,
		PacketConn: conn,
		done:       make(chan struct{}),
		pConnDone:  make(chan string, 100),
		pConns:     make(map[string]*PFCPConn),
		upf:        upf,
		metrics:    metrics,
	}

	node.collector = NewPFCPNodeCollector(node)

	return node
}

func (node *PFCPNode) handleNewPeers() {
	lAddrStr := node.LocalAddr().String()
	log.Infoln("listening for new PFCP connections on", lAddrStr)

	for {
		buf := make([]byte, 1024)

		n, rAddr, err := node.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			continue
		}

		rAddrStr := rAddr.String()

		_, ok := node.pConns[rAddrStr]
		if ok {
			log.Warnln("Drop packet for existing PFCPconn received from", rAddrStr)
			continue
		}

		node.NewPFCPConn(lAddrStr, rAddrStr, buf[:n])
	}
}

// Serve listens for the first packet from a new PFCP peer and creates PFCPConn.
func (node *PFCPNode) Serve() {
	go node.handleNewPeers()

	shutdown := false

	for !shutdown {
		select {
		case fseid := <-node.upf.reportNotifyChan:
			// TODO: Logic to distinguish PFCPConn based on SEID
			for _, pConn := range node.pConns {
				pConn.handleDigestReport(fseid)
				break
			}
		case rAddr := <-node.pConnDone:
			delete(node.pConns, rAddr)
			log.Infoln("Removed connection to", rAddr)
		case <-node.ctx.Done():
			shutdown = true

			log.Infoln("Entering node shutdown")

			err := node.Close()
			if err != nil {
				log.Errorln("Error closing PFCPNode conn", err)
			}

			// Clear out the remaining pconn completions
			for len(node.pConns) > 0 {
				rAddr := <-node.pConnDone
				delete(node.pConns, rAddr)
				log.Infoln("Removed connection to", rAddr)
			}

			close(node.pConnDone)
			log.Infoln("Done waiting for PFCPConn completions")
		}
	}

	close(node.done)
}

// Done waits for Shutdown() to complete
func (node *PFCPNode) Done() {
	<-node.done
	log.Infoln("Shutdown complete")
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
	c := &PfcpNodeCollector{
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

	prometheus.MustRegister(c)

	return c
}

func (col PfcpNodeCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(col, ch)
}

func (col PfcpNodeCollector) Collect(ch chan<- prometheus.Metric) {
	stats, err := col.node.upf.sessionStats2()
	if err != nil {
		log.Errorln(err)
		return
	}
	// TODO: pick first connection for now
	var con *PFCPConn
	for _, c := range col.node.pConns {
		con = c
		break
	}

	for _, s := range stats {
		fseidString := strconv.FormatUint(s.Fseid, 10)
		pdrString := strconv.FormatUint(s.Pdr, 10)
		ueIpString := "unknown"

		if con != nil {
			session, ok := con.sessions[s.Fseid]
			if !ok {
				log.Errorln("Invalid or unknown FSEID in session info", s)
				continue
			}
			for _, p := range session.pdrs {
				if uint64(p.pdrID) != s.Pdr {
					continue
				}
				// Only downlink PDRs contain the UE address.
				if p.srcIP > 0 {
					ueIpString = int2ip(p.srcIP).String()
					log.Warnln(p.fseID, " -> ", ueIpString)
					break
				}
			}
		} else {
			log.Warnln("No active PFCP connection, IP lookup disabled")
		}

		ch <- prometheus.MustNewConstMetric(
			col.sessionTxPackets,
			prometheus.GaugeValue,
			float64(s.TxPackets), // check if uint possible
			fseidString,
			pdrString,
			ueIpString,
		)
		ch <- prometheus.MustNewConstMetric(
			col.sessionRxPackets,
			prometheus.GaugeValue,
			float64(s.RxPackets),
			fseidString,
			pdrString,
			ueIpString,
		)
		ch <- prometheus.MustNewConstMetric(
			col.sessionTxBytes,
			prometheus.GaugeValue,
			float64(s.TxBytes),
			fseidString,
			pdrString,
			ueIpString,
		)
		ch <- prometheus.MustNewConstSummary(
			col.sessionLatency,
			s.TxPackets,
			0,
			s.Latency,
			fseidString,
			pdrString,
			ueIpString,
		)
		ch <- prometheus.MustNewConstSummary(
			col.sessionJitter,
			s.TxPackets,
			0,
			s.Jitter,
			fseidString,
			pdrString,
			ueIpString,
		)
	}
}
