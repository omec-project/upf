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
	"github.com/wmnsk/go-pfcp/message"
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

// PFCPConn represents a PFCP connection with a unique PFCP peer.
type PFCPConn struct {
	ctx context.Context
	// child socket for all subsequent packets from an "established PFCP connection"
	net.Conn
	seqNum sequenceNumber
	mgr    *PFCPSessionMgr
	upf    *upf
	// channel to signal PFCPNode on exit
	done chan<- string
}

// NewPFCPConn creates a connected UDP socket to the rAddr PFCP peer specified.
func NewPFCPConn(ctx context.Context, upf *upf, done chan<- string, lAddr, rAddr string) *PFCPConn {
	conn, err := reuse.Dial("udp", lAddr, rAddr)
	if err != nil {
		log.Errorln("dial socket failed", err)
	}

	log.Infoln("Created PFCPConn for", conn.RemoteAddr().String())

	return &PFCPConn{
		ctx:  ctx,
		Conn: conn,
		mgr:  NewPFCPSessionMgr(100),
		upf:  upf,
		done: done,
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

	<-pConn.ctx.Done()
	pConn.Shutdown()
}

// Shutdown stops connection backing PFCPConn.
func (pConn *PFCPConn) Shutdown() error {
	pConn.done <- pConn.LocalAddr().String()

	err := pConn.Close()
	if err != nil {
		return err
	}

	log.Infoln("PFCPConn: Shutdown complete", pConn.RemoteAddr().String())
	return nil
}

// HandlePFCPMsg handles different types of PFCP messages.
func (pConn *PFCPConn) HandlePFCPMsg(buf []byte) {
	var outgoingMessage []byte

	msg, err := message.Parse(buf)
	if err != nil {
		log.Errorln("Ignoring undecodable message: ", buf, " error: ", err)
		return
	}

	switch msg.MessageType() {
	case message.MsgTypeAssociationSetupRequest:
		// Cleanup my session
		outgoingMessage = pConn.handleAssociationSetupRequest(msg)
	case message.MsgTypeAssociationSetupResponse:
		pConn.handleAssociationSetupResponse(msg)
		// start heartbeats
	case message.MsgTypePFDManagementRequest:
		outgoingMessage = pConn.handlePFDMgmtRequest(msg)
	case message.MsgTypeSessionEstablishmentRequest:
		outgoingMessage = pConn.handleSessionEstablishmentRequest(msg)
	case message.MsgTypeSessionModificationRequest:
		outgoingMessage = pConn.handleSessionModificationRequest(msg)
	case message.MsgTypeHeartbeatRequest:
		outgoingMessage = pConn.handleHeartbeatRequest(msg)
	case message.MsgTypeSessionDeletionRequest:
		outgoingMessage = pConn.handleSessionDeletionRequest(msg)
	case message.MsgTypeAssociationReleaseRequest:
		outgoingMessage = pConn.handleAssociationReleaseRequest(msg)
		// Cleanup my sessions
	case message.MsgTypeSessionReportResponse:
		pConn.handleSessionReportResponse(msg)
	default:
		log.Errorln("Message type: ", msg.MessageTypeName(), " is currently not supported")
		return
	}

	// send the response out
	if outgoingMessage != nil {
		if _, err := pConn.Write(outgoingMessage); err != nil {
			log.Errorln("Unable to transmit association setup response", err)
		}
	}
}

func (c *PFCPConn) getSeqNum() uint32 {
	c.seqNum.mux.Lock()
	defer c.seqNum.mux.Unlock()
	c.seqNum.seq++

	return c.seqNum.seq
}
