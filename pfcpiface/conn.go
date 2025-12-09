// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	reuse "github.com/libp2p/go-reuseport"
	"github.com/wmnsk/go-pfcp/ie"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

const (
	PFCPPort = "8805"
	MaxItems = 10
)

// Timeout : connection timeout.
var Timeout = 1000 * time.Millisecond

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

type recoveryTS struct {
	local  time.Time
	remote time.Time
}

type nodeID struct {
	localIE *ie.IE
	local   string
	remote  string
}

// PFCPConn represents a PFCP connection with a unique PFCP peer.
type PFCPConn struct {
	ctx context.Context
	// child socket for all subsequent packets from an "established PFCP connection"
	net.Conn
	ts         recoveryTS
	seqNum     sequenceNumber
	rng        *rand.Rand
	maxRetries int
	appPFDs    map[string]appPFD

	store SessionsStore

	nodeID nodeID
	upf    *upf
	// channel to signal PFCPNode on exit
	done     chan<- string
	shutdown chan struct{}

	metrics.InstrumentPFCP

	hbReset     chan struct{}
	hbCtxCancel context.CancelFunc

	pendingReqs sync.Map

	shutdownOnce sync.Once
	isShutdown   atomic.Bool
}

func (pConn *PFCPConn) startHeartBeatMonitor() {
	// Check if already shutdown
	if pConn.IsShutdown() {
		return
	}

	// Stop HeartBeat routine if already running
	if pConn.hbCtxCancel != nil {
		pConn.hbCtxCancel()
		pConn.hbCtxCancel = nil
	}

	hbCtx, hbCancel := context.WithCancel(pConn.ctx)
	pConn.hbCtxCancel = hbCancel

	logger.PfcpLog.With("interval", pConn.upf.hbInterval).Infoln("starting Heartbeat timer")

	heartBeatExpiryTimer := time.NewTicker(pConn.upf.hbInterval)

	for {
		select {
		case <-hbCtx.Done():
			logger.PfcpLog.Infoln("cancel HeartBeat Timer", pConn.RemoteAddr().String())
			heartBeatExpiryTimer.Stop()
			return
		case <-pConn.hbReset:
			// Check if shutdown before resetting timer
			if pConn.IsShutdown() {
				heartBeatExpiryTimer.Stop()
				return
			}
			heartBeatExpiryTimer.Reset(pConn.upf.hbInterval)
		case <-heartBeatExpiryTimer.C:
			// Check if shutdown before sending heartbeat
			if pConn.IsShutdown() {
				heartBeatExpiryTimer.Stop()
				return
			}

			logger.PfcpLog.Debugln("HeartBeat Interval Timer Expired", pConn.RemoteAddr().String())

			r := pConn.getHeartBeatRequest()

			if _, timeout := pConn.sendPFCPRequestMessage(r); timeout {
				heartBeatExpiryTimer.Stop()
				pConn.Shutdown()
			}
		}
	}
}

// NewPFCPConn creates a connected UDP socket to the rAddr PFCP peer specified.
// buf is the first message received from the peer, nil if we are initiating.
func (node *PFCPNode) NewPFCPConn(lAddr, rAddr string, buf []byte) *PFCPConn {
	conn, err := reuse.Dial("udp", lAddr, rAddr)
	if err != nil {
		logger.PfcpLog.Errorln("dial socket failed", err)
	}

	ts := recoveryTS{
		local: time.Now(),
	}

	// TODO: Get SEID range from PFCPNode for this PFCPConn
	logger.PfcpLog.Infoln("created PFCPConn from:", conn.LocalAddr(), "to:", conn.RemoteAddr())

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404

	var p = &PFCPConn{
		ctx:            node.ctx,
		Conn:           conn,
		ts:             ts,
		rng:            rng,
		maxRetries:     100,
		store:          NewInMemoryStore(),
		upf:            node.upf,
		done:           node.pConnDone,
		shutdown:       make(chan struct{}),
		InstrumentPFCP: node.metrics,
		hbReset:        make(chan struct{}, 100),
		hbCtxCancel:    nil,
	}

	p.setLocalNodeID(node.upf.nodeID)

	if buf != nil {
		// TODO: Check if the first msg is Association Setup Request
		p.HandlePFCPMsg(buf)
	}

	// Update map of connections
	node.pConns.Store(rAddr, p)

	go p.Serve()

	return p
}

func (pConn *PFCPConn) setLocalNodeID(id string) {
	nodeIP := net.ParseIP(id)

	// NodeID - FQDN
	if id != "" && nodeIP == nil {
		pConn.nodeID.localIE = ie.NewNodeID("", "", id)
		pConn.nodeID.local = id

		return
	}

	// NodeID provided is not an IP, use local address
	if nodeIP == nil {
		nodeIP = pConn.LocalAddr().(*net.UDPAddr).IP
	}

	pConn.nodeID.local = nodeIP.String()

	// NodeID - IPv4 vs IPv6
	if nodeIP.To4() != nil {
		pConn.nodeID.localIE = ie.NewNodeID(pConn.nodeID.local, "", "")
	} else {
		pConn.nodeID.localIE = ie.NewNodeID("", pConn.nodeID.local, "")
	}
}

// Serve serves forever a single PFCP peer.
func (pConn *PFCPConn) Serve() {
	connTimeout := make(chan struct{}, 1)
	go func(connTimeout chan struct{}) {
		recvBuf := make([]byte, 65507) // Maximum UDP payload size

		for {
			// Check if shutdown before attempting to read
			if pConn.IsShutdown() {
				return
			}

			err := pConn.SetReadDeadline(time.Now().Add(pConn.upf.readTimeout))
			if err != nil {
				logger.PfcpLog.Errorf("failed to set read timeout: %v", err)
			}

			n, err := pConn.Read(recvBuf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					logger.PfcpLog.Infof("read timeout for connection %v<->%v, is the SMF still alive?",
						pConn.LocalAddr(), pConn.RemoteAddr())
					connTimeout <- struct{}{}
					return
				}

				if errors.Is(err, net.ErrClosed) {
					return
				}

				continue
			}

			// Check again before processing the message
			if pConn.IsShutdown() {
				return
			}

			buf := append([]byte{}, recvBuf[:n]...)
			pConn.HandlePFCPMsg(buf)
		}
	}(connTimeout)

	// TODO: Sender goroutine

	for {
		select {
		case <-connTimeout:
			pConn.Shutdown()
			return
		case <-pConn.ctx.Done():
			pConn.Shutdown()
			return

		case <-pConn.shutdown:
			return
		}
	}
}

// Shutdown stops connection backing PFCPConn.
func (pConn *PFCPConn) Shutdown() {
	pConn.shutdownOnce.Do(func() {
		pConn.executeShutdown()
	})
}

func (pConn *PFCPConn) executeShutdown() {
	// Mark as shutdown atomically
	pConn.isShutdown.Store(true)

	close(pConn.shutdown)

	if pConn.hbCtxCancel != nil {
		pConn.hbCtxCancel()
		pConn.hbCtxCancel = nil
	}

	// Cleanup all sessions in this conn
	for _, sess := range pConn.store.GetAllSessions() {
		pConn.upf.SendMsgToUPF(upfMsgTypeDel, sess.PacketForwardingRules, PacketForwardingRules{})
		pConn.RemoveSession(sess)
	}

	rAddr := pConn.RemoteAddr().String()

	// Safely send to done channel (non-blocking)
	select {
	case pConn.done <- rAddr:
	default:
		// Channel might be full or closed, do not block
		logger.PfcpLog.Warnln("could not send shutdown notification for", rAddr)
	}

	err := pConn.Close()
	if err != nil {
		logger.PfcpLog.Errorln("failed to close PFCP connection")
		return
	}

	logger.PfcpLog.Infoln("shutdown complete for", rAddr)
}

// IsShutdown returns true if the connection has been shutdown
func (pConn *PFCPConn) IsShutdown() bool {
	return pConn.isShutdown.Load()
}

func (pConn *PFCPConn) getSeqNum() uint32 {
	pConn.seqNum.mux.Lock()
	defer pConn.seqNum.mux.Unlock()
	pConn.seqNum.seq++

	return pConn.seqNum.seq
}
