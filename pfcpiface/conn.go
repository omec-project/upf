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
	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/wmnsk/go-pfcp/ie"
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

	nodeID   nodeID
	upf      *upf
	node     *PFCPNode // Reference to parent node for WaitGroup management
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

	p := &PFCPConn{
		ctx:            node.ctx,
		Conn:           conn,
		ts:             ts,
		rng:            rng,
		maxRetries:     100,
		store:          NewInMemoryStore(),
		upf:            node.upf,
		node:           node,
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

	// Register this connection with the WaitGroup
	node.connWg.Add(1)

	// Update map of connections
	node.pConns.Store(rAddr, p)

	// Start the connection with proper cleanup
	go func() {
		defer func() {
			// Signal completion to WaitGroup
			node.connWg.Done()
			// Remove from map when done
			node.pConns.Delete(rAddr)
			logger.PfcpLog.Infoln("removed connection to", rAddr)
		}()

		p.Serve()
	}()

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
	pConn.waitForShutdown(connTimeout)
}

func (pConn *PFCPConn) waitForShutdown(connTimeout chan struct{}) {
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
	var unfinished []PFCPSession

	for _, sess := range pConn.store.GetAllSessions() {
		// A removal that ran out of time may have left rules matching the session's
		// address, and once the session is forgotten nothing names them. So the address
		// is released only once a removal of its rules has finished; the sessions whose
		// removal did not are retried together below.
		if _, finished := pConn.upf.SendMsgToUPFWithCompletion(
			upfMsgTypeDel, sess.PacketForwardingRules, PacketForwardingRules{},
		); !finished {
			unfinished = append(unfinished, sess)
			continue
		}

		// The pool is built in NewUPF and belongs to the upf, which every connection
		// shares, so an address not returned here is lost for the life of the process:
		// the session is about to be forgotten and its local SEID will never be
		// presented again. RemoveSession only drops the metrics and the store entry.
		pConn.upf.ippool.Release(sess.localSEID)

		pConn.RemoveSession(sess)
	}

	pConn.retryUnfinishedRemovals(unfinished)

	rAddr := pConn.RemoteAddr().String()

	err := pConn.Close()
	if err != nil {
		logger.PfcpLog.Errorln("failed to close PFCP connection")
		return
	}

	logger.PfcpLog.Infoln("shutdown complete for", rAddr)
}

// retryUnfinishedRemovals makes one more attempt at removing the rules of the sessions
// whose removal ran out of time at teardown. Each session is retried in a batch of its
// own and all of them at once, so the retry adds one batch's timeout to the teardown
// rather than one per session, and a session whose rules drained is not held back by
// another's that did not. A rule the late removal already took out is answered ENOENT,
// which counts as done, so a removal that merely landed late finishes here.
//
// The addresses of sessions whose rules are still not known to be gone stay held:
// releasing them would let the pool give another UE, on any association, an address a
// surviving rule still matches, and nothing is left that could remove that rule. They
// are lost until the process restarts, which clears the datapath.
//
// A removal the datapath refused is released, here as on the first pass. What refuses
// one today is a rule that could not be translated or marshalled, or an EINVAL about
// its shape that the rule's own add would have met first -- so it was never programmed,
// and holding its address would lose it for nothing.
func (pConn *PFCPConn) retryUnfinishedRemovals(sessions []PFCPSession) {
	finished := make([]bool, len(sessions))

	var wg sync.WaitGroup

	for i, s := range sessions {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, finished[i] = pConn.upf.SendMsgToUPFWithCompletion(
				upfMsgTypeDel, s.PacketForwardingRules, PacketForwardingRules{})
		}()
	}

	wg.Wait()

	held := 0

	for i, s := range sessions {
		if finished[i] {
			pConn.upf.ippool.Release(s.localSEID)
		} else {
			held++
		}

		pConn.RemoveSession(s)
	}

	if held > 0 {
		logger.PfcpLog.Warnln("the removal of", held, "session(s) did not finish at "+
			"teardown, twice; holding their UE addresses until the process restarts")
	}
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
