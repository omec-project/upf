// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation
// Copyright 2021 Open Networking Foundation
package pfcpiface

import (
	"context"
	"errors"
	"net"

	reuse "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

const (
	maxPFCPConns = 100
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
	pConns map[string]*PFCPConn
	// upf
	upf *upf
	// metrics for PFCP messages and sessions
	metrics metrics.InstrumentPFCP
	// PFCP Connection Ids Pool
	pfcpConnIdPool []uint32
}

func (node *PFCPNode) initPFCPConnIdPool() {
	for i := 1; i < maxPFCPConns+1; i++ {
		node.pfcpConnIdPool = append(node.pfcpConnIdPool, uint32(i))
	}
}

func (node *PFCPNode) allocatePFCPConnId() (uint32, error) {
	if len(node.pfcpConnIdPool) == 0 {
		return 0, ErrOperationFailedWithReason("Allocation of PFCPConn Ids", "no free pfcpconn ids available")
	}

	allocatedId := node.pfcpConnIdPool[0]
	node.pfcpConnIdPool = node.pfcpConnIdPool[1:]

	return allocatedId, nil
}

func (node *PFCPNode) cleanUpPFCPConn(rAddr string) {
	pConn := node.pConns[rAddr]
	if pConn != nil {
		node.pfcpConnIdPool = append(node.pfcpConnIdPool, pConn.pConnId)
		delete(node.pConns, rAddr)
	}
}

// NewPFCPNode create a new PFCPNode listening on local address.
func NewPFCPNode(upf *upf) *PFCPNode {
	conn, err := reuse.ListenPacket("udp", ":"+PFCPPort)
	if err != nil {
		log.Fatalln("ListenUDP failed", err)
	}

	metrics, err := metrics.NewPrometheusService()
	if err != nil {
		log.Fatalln("prom metrics service init failed", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &PFCPNode{
		ctx:            ctx,
		cancel:         cancel,
		PacketConn:     conn,
		done:           make(chan struct{}),
		pConnDone:      make(chan string, 100),
		pConns:         make(map[string]*PFCPConn),
		upf:            upf,
		metrics:        metrics,
		pfcpConnIdPool: make([]uint32, 0, maxPFCPConns),
	}

	node.initPFCPConnIdPool()

	return node
}

func (node *PFCPNode) tryConnectToN4Peers(lAddrStr string) {
	for _, peer := range node.upf.peers {
		conn, err := net.Dial("udp", peer+":"+PFCPPort)
		if err != nil {
			log.Warnln("Failed to establish PFCP connection to peer ", peer)
			continue
		}

		remoteAddr := conn.RemoteAddr().(*net.UDPAddr)
		n4DstIP := remoteAddr.IP

		log.WithFields(log.Fields{
			"SPGWC/SMF host": peer,
			"CP node":        n4DstIP.String(),
		}).Info("Establishing PFCP Conn with CP node")

		pfcpConn := node.NewPFCPConn(lAddrStr, n4DstIP.String()+":"+PFCPPort, nil)
		if pfcpConn != nil {
			go pfcpConn.sendAssociationRequest()
		}
	}
}

func (node *PFCPNode) handleNewPeers() {
	lAddrStr := node.LocalAddr().String()
	log.Infoln("listening for new PFCP connections on", lAddrStr)

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
			pConn := node.findPFCPConnByFseid(fseid)
			if pConn != nil {
				go pConn.handleDigestReport(fseid)
			}
		case rAddr := <-node.pConnDone:
			node.cleanUpPFCPConn(rAddr)
			log.Infoln("Removed connection to", rAddr)
		case <-node.ctx.Done():
			shutdown = true

			log.Infoln("Shutting down PFCP node")

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

			node.upf.exit()
		}
	}

	close(node.done)
}

func (node *PFCPNode) Stop() {
	node.cancel()

	if err := node.metrics.Stop(); err != nil {
		// TODO: propagate error upwards
		log.Errorln(err)
	}
}

// Done waits for Shutdown() to complete
func (node *PFCPNode) Done() {
	<-node.done
	log.Infoln("Shutdown complete")
}

func (node *PFCPNode) findPFCPConnByFseid(fseid uint64) *PFCPConn {
	connID := uint32(fseid >> 32)

	for _, pConn := range node.pConns {
		if pConn.pConnId == connID {
			return pConn
		}

		continue
	}

	return nil
}
