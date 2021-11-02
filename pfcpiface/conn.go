// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"errors"
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

// PFCPConn represents a PFCP connection with a unique PFCP peer.
type PFCPConn struct {
	ctx context.Context
	// child socket for all subsequent packets from an "established PFCP connection"
	net.Conn
	ts     recoveryTS
	seqNum sequenceNumber
	mgr    *PFCPSessionMgr
	upf    *upf
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

	log.Infoln("Created PFCPConn for", conn.RemoteAddr().String())

	ts := recoveryTS{
		local: time.Now(),
	}

	return &PFCPConn{
		ctx:      ctx,
		Conn:     conn,
		ts:       ts,
		mgr:      NewPFCPSessionMgr(100),
		upf:      upf,
		done:     done,
		shutdown: make(chan struct{}),
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
	for seid, sess := range pConn.mgr.sessions {
		pConn.upf.sendMsgToUPF(upfMsgTypeDel, sess.pdrs, sess.fars, sess.qers)
		pConn.mgr.RemoveSession(seid)
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

func (c *PFCPConn) getSeqNum() uint32 {
	c.seqNum.mux.Lock()
	defer c.seqNum.mux.Unlock()
	c.seqNum.seq++

	return c.seqNum.seq
}
