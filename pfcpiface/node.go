// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation
// Copyright 2021 Open Networking Foundation
package pfcpiface

import (
	"context"
	"errors"
	"net"
	"sync"

	reuse "github.com/libp2p/go-reuseport"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

// PFCPNode represents a PFCP endpoint of the UPF.
type PFCPNode struct {
	ctx    context.Context
	cancel context.CancelFunc
	// listening socket for new "PFCP connections"
	net.PacketConn
	// done is closed to signal shutdown complete
	done chan struct{}
	// channel for PFCPConn to signal exit by sending their remote address
	pConnDone chan string
	// map of existing connections
	pConns sync.Map
	// upf
	upf *upf
	// metrics for PFCP messages and sessions
	metrics metrics.InstrumentPFCP
}

// NewPFCPNode create a new PFCPNode listening on local address.
func NewPFCPNode(upf *upf) *PFCPNode {
	conn, err := reuse.ListenPacket("udp",
		upf.n4addr+":"+PFCPPort)
	if err != nil {
		logger.PfcpLog.Fatalln("listen UDP failed", err)
	}

	metrics, err := metrics.NewPrometheusService()
	if err != nil {
		logger.PfcpLog.Fatalln("prom metrics service init failed", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &PFCPNode{
		ctx:        ctx,
		cancel:     cancel,
		PacketConn: conn,
		done:       make(chan struct{}),
		pConnDone:  make(chan string, 100),
		upf:        upf,
		metrics:    metrics,
	}
}

func (node *PFCPNode) tryConnectToN4Peers(lAddrStr string) {
	for _, peer := range node.upf.peers {
		conn, err := net.Dial("udp", peer+":"+PFCPPort)
		if err != nil {
			logger.PfcpLog.Warnln("failed to establish PFCP connection to peer", peer)
			continue
		}

		remoteAddr := conn.RemoteAddr().(*net.UDPAddr)
		n4DstIP := remoteAddr.IP

		logger.PfcpLog.Infof("Establishing PFCP Conn with CP node. SPGWC/SMF host: %s, CP node: %s", peer, n4DstIP.String())

		pfcpConn := node.NewPFCPConn(lAddrStr, n4DstIP.String()+":"+PFCPPort, nil)
		if pfcpConn != nil {
			go pfcpConn.sendAssociationRequest()
		}
	}
}

func (node *PFCPNode) handleNewPeers() {
	lAddrStr := node.LocalAddr().String()
	logger.PfcpLog.Infoln("listening for new PFCP connections on", lAddrStr)

	node.tryConnectToN4Peers(lAddrStr)

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

		_, ok := node.pConns.Load(rAddrStr)
		if ok {
			logger.PfcpLog.Warnln("drop packet for existing PFCPconn received from", rAddrStr)
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
			node.pConns.Range(func(key, value interface{}) bool {
				pConn := value.(*PFCPConn)
				pConn.handleDigestReport(fseid)
				return false
			})
		case rAddr := <-node.pConnDone:
			node.pConns.Delete(rAddr)
			logger.PfcpLog.Infoln("removed connection to", rAddr)
		case <-node.ctx.Done():
			shutdown = true

			logger.PfcpLog.Infoln("shutting down PFCP node")

			err := node.Close()
			if err != nil {
				logger.PfcpLog.Errorln("error closing PFCPNode conn", err)
			}

			// Clear out the remaining pconn completions
		clearLoop:
			for {
				select {
				case rAddr, ok := <-node.pConnDone:
					{
						if !ok {
							// channel is closed, break
							break clearLoop
						}
						node.pConns.Delete(rAddr)
						logger.PfcpLog.Infoln("removed connection to", rAddr)
					}
				default:
					// nothing to read from channel
					break clearLoop
				}
			}

			if len(node.pConnDone) > 0 {
				for rAddr := range node.pConnDone {
					node.pConns.Delete(rAddr)
					logger.PfcpLog.Infoln("removed connection to", rAddr)
				}
			}

			close(node.pConnDone)
			logger.PfcpLog.Infoln("done waiting for PFCPConn completions")

			node.upf.Exit()
		}
	}

	close(node.done)
}

func (node *PFCPNode) Stop() {
	node.cancel()

	if err := node.metrics.Stop(); err != nil {
		// TODO: propagate error upwards
		logger.PfcpLog.Errorln(err)
	}
}

// Done waits for Shutdown() to complete
func (node *PFCPNode) Done() {
	<-node.done
	logger.PfcpLog.Infoln("shutdown complete")
}
