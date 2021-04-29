// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wmnsk/go-pfcp/message"
)

// PktBufSz : buffer size for incoming pkt
const (
	PktBufSz    = 1500
	PFCPPort    = "8805"
	MaxItems    = 10
	readTimeout = 25 * time.Second
)

//Timeout : connection timeout
var Timeout = 1000 * time.Millisecond

// PFCPConn represents a PFCP connection
type PFCPConn struct {
	seqNum sequenceNumber
	mgr    *PFCPSessionMgr
}

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

func (c *PFCPConn) getSeqNum() uint32 {
	c.seqNum.mux.Lock()
	defer c.seqNum.mux.Unlock()
	c.seqNum.seq++
	return c.seqNum.seq
}

func pfcpifaceMainLoop(upf *upf, accessIP, coreIP, sourceIP, smfName string) {
	var pconn PFCPConn
	pconn.mgr = NewPFCPSessionMgr(100)
	rTimeout := readTimeout
	if upf.readTimeout != 0 {
		rTimeout = time.Duration(upf.readTimeout)
	}
	if upf.connTimeout != 0 {
		Timeout = upf.connTimeout
	}

	log.Println("timeout : ", Timeout, ", readTimeout : ", rTimeout)
	log.Println("pfcpifaceMainLoop@" + upf.fqdnHost + " says hello!!!")

	cpConnectionStatus := make(chan bool)

	// Verify IP + Port binding
	laddr, err := net.ResolveUDPAddr("udp", sourceIP+":"+PFCPPort)
	if err != nil {
		log.Fatalln("Unable to resolve udp addr!", err)
		return
	}

	// Listen on the port
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatalln("Unable to bind to listening port!", err)
		return
	}

	// flag to check SMF/SPGW-C is connected
	// cpConnected is true if upf received request from control plane or
	// if upf receives +ve response for upf initiated setup request
	cpConnected := false

	// cleanup the pipeline
	cleanupSessions := func() {
		if upf.simInfo != nil {
			return
		}
		sendDeleteAllSessionsMsgtoUPF(upf)
		cpConnected = false
	}
	// initiate connection if smf address available
	log.Println("calling manageSmfConnection smf service name ", smfName)
	manageConnection := false
	if smfName != "" {
		manageConnection = true
		go pconn.manageSmfConnection(upf.nodeIP.String(), accessIP, smfName, conn, cpConnectionStatus, upf.recoveryTime)
	}

	// Initialize pkt buf
	buf := make([]byte, PktBufSz)
	// Initialize pkt header

	for {
		err := conn.SetReadDeadline(time.Now().Add(rTimeout))
		if err != nil {
			log.Fatalln("Unable to set deadline for read:", err)
		}
		// blocking read
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				// do nothing for the time being
				log.Println(err)
				cpConnected = false
				if manageConnection {
					cpConnectionStatus <- cpConnected
				}
				cleanupSessions()
				continue
			}
			log.Fatalln("Read error:", err)
		}

		// use wmnsk lib to parse the pfcp message
		msg, err := message.Parse(buf[:n])
		if err != nil {
			log.Println("Ignoring undecodable message: ", buf[:n], " error: ", err)
			continue
		}

		// if sourceIP is not set, fetch it from the msg header
		if sourceIP == "0.0.0.0" {
			addrString := strings.Split(addr.String(), ":")
			sourceIP = getLocalIP(addrString[0]).String()
			log.Println("Source IP address is now: ", sourceIP)
		}

		// if nodeIP is not set, fetch it from the msg header
		if upf.nodeIP.String() == "0.0.0.0" {
			addrString := strings.Split(addr.String(), ":")
			upf.nodeIP = getLocalIP(addrString[0])
			log.Println("Node IP address is now: ", upf.nodeIP.String())
		}

		// log.Println("Message: ", msg)

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			cleanupSessions()
			upf.setInfo(conn, addr, &pconn)
			outgoingMessage = pconn.handleAssociationSetupRequest(upf, msg, addr, sourceIP, accessIP, coreIP)
			if outgoingMessage != nil {
				cpConnected = true
				if manageConnection {
					// if we initiated connection, inform go routine
					cpConnectionStatus <- cpConnected
				}
			}
		case message.MsgTypeAssociationSetupResponse:
			cpConnected = handleAssociationSetupResponse(msg, addr, sourceIP, accessIP)
			if manageConnection {
				// pass on information to go routine that result of association response
				cpConnectionStatus <- cpConnected
			}
		case message.MsgTypePFDManagementRequest:
			outgoingMessage = pconn.handlePFDMgmtRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = pconn.handleSessionEstablishmentRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = pconn.handleSessionModificationRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr, upf.recoveryTime)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = pconn.handleSessionDeletionRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(upf, msg, addr, sourceIP, accessIP, upf.recoveryTime)
			cleanupSessions()
		default:
			log.Println("Message type: ", msg.MessageTypeName(), " is currently not supported")
			continue
		}

		// send the response out
		if outgoingMessage != nil {
			if _, err := conn.WriteTo(outgoingMessage, addr); err != nil {
				log.Fatalln("Unable to transmit association setup response", err)
			}
		}

	}
}
