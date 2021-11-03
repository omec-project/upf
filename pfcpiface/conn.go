// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"time"

	reuse "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"
)

// PktBufSz : buffer size for incoming pkt.
const (
	PktBufSz    = 1500
	PFCPPort    = "8805"
	MaxItems    = 10
	readTimeout = 25 * time.Second
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
	local  string
	remote string
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
	sessions   map[uint64]*PFCPSession
	nodeID     nodeID
	upf        *upf
	// channel to signal PFCPNode on exit
	done     chan<- string
	shutdown chan struct{}
}

// NewPFCPConn creates a connected UDP socket to the rAddr PFCP peer specified.
func NewPFCPConn(ctx context.Context, upf *upf, done chan<- string, lAddr, rAddr string) *PFCPConn {
	conn, err := reuse.Dial("udp", lAddr, rAddr)
	if err != nil {
		log.Errorln("dial socket failed", err)
	}

	ts := recoveryTS{
		local: time.Now(),
	}

	log.Infoln("Created PFCPConn from:", conn.LocalAddr(), "to:", conn.RemoteAddr())

	return &PFCPConn{
		ctx:        ctx,
		Conn:       conn,
		ts:         ts,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		maxRetries: 100,
		sessions:   make(map[uint64]*PFCPSession),
		upf:        upf,
		done:       done,
		shutdown:   make(chan struct{}),
	}
}

// Serve serves forever a single PFCP peer.
func (pConn *PFCPConn) Serve() {
	go func() {
		for {
			buf := make([]byte, 1024)

			n, err := pConn.Read(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				continue
			}

			pConn.HandlePFCPMsg(buf[:n])
		}
	}()

	// TODO: Sender goroutine

	for {
		select {
		case <-pConn.ctx.Done():
			pConn.Shutdown()
			return
		case <-pConn.shutdown:
			return
		}
	}
}

// Shutdown stops connection backing PFCPConn.
func (pConn *PFCPConn) Shutdown() error {
	close(pConn.shutdown)

	// Cleanup all sessions in this conn
	for seid, sess := range pConn.sessions {
		pConn.upf.sendMsgToUPF(upfMsgTypeDel, sess.pdrs, sess.fars, sess.qers)
		pConn.RemoveSession(seid)
	}

	rAddr := pConn.RemoteAddr().String()
	pConn.done <- rAddr

	err := pConn.Close()
	if err != nil {
		return err
	}

	log.Infoln("Shutdown complete for", rAddr)
	return nil
}

func (pConn *PFCPConn) getSeqNum() uint32 {
	pConn.seqNum.mux.Lock()
	defer pConn.seqNum.mux.Unlock()
	pConn.seqNum.seq++

	return pConn.seqNum.seq
}
