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
	Timeout     = 1000 * time.Millisecond
	readTimeout = 25 * time.Second
)

// PFCPConn represents a PFCP connection
type PFCPConn struct {
	seqNum sequenceNumber
	mgr    *PFCPSessionMgr
}

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

type rcvdPacket struct {
	Buf      [1500]byte
	Pkt_size int
	Address  net.Addr
}

type parsedPacket struct {
	Msg     *message.Message
	Address net.Addr
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
		go pconn.manageSmfConnection(sourceIP, accessIP, smfName, conn, cpConnectionStatus, upf.recoveryTime)
	}

	// Initialize pkt buf
	pfcpRcvdPktsChan := make(chan *rcvdPacket, 1000)
	pfcpParsedPktsChan := make(chan *parsedPacket, 1000)
	// Initialize pkt header

	pfcpPacketReader := func(pfcpRcvdPkts chan *rcvdPacket) {

		for {
			err := conn.SetReadDeadline(time.Now().Add(readTimeout))
			if err != nil {
				log.Fatalln("Unable to set deadline for read:", err)
			}
			pkt := new(rcvdPacket)
			// blocking read
			n, addr, err := conn.ReadFrom(pkt.Buf[:1500])
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
			pkt.Pkt_size = n
			pkt.Address = addr
			pfcpRcvdPktsChan <- pkt
		}
	}

	pfcpPacketParsing := func(pkt *rcvdPacket, pfcpParsedPktsChan chan *parsedPacket) {

		// use wmnsk lib to parse the pfcp message
		msg, err := message.Parse(pkt.Buf[:pkt.Pkt_size])
		if err != nil {
			log.Println("Ignoring undecodable message size ", pkt.Pkt_size)
			log.Println("Ignoring undecodable message: ", pkt.Buf[:pkt.Pkt_size], " error: ", err)
			return
		}

		// if sourceIP is not set, fetch it from the msg header
		if sourceIP == "0.0.0.0" {
			addrString := strings.Split(pkt.Address.String(), ":")
			sourceIP = getLocalIP(addrString[0]).String()
			log.Println("Source IP address is now: ", sourceIP)
		}
		pPkt := new(parsedPacket)
		pPkt.Msg = &msg
		pPkt.Address = pkt.Address
		pfcpParsedPktsChan <- pPkt
	}

	// log.Println("Message: ", msg)
	pfcpPacketProcessing := func(pPkt *parsedPacket) {

		msg := *pPkt.Msg

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			cleanupSessions()
			upf.setInfo(conn, pPkt.Address, &pconn)
			outgoingMessage = pconn.handleAssociationSetupRequest(upf, msg, pPkt.Address, sourceIP, accessIP, coreIP)
			if outgoingMessage != nil {
				cpConnected = true
				if manageConnection {
					// if we initiated connection, inform go routine
					cpConnectionStatus <- cpConnected
				}
			}
		case message.MsgTypeAssociationSetupResponse:
			cpConnected = handleAssociationSetupResponse(msg, pPkt.Address, sourceIP, accessIP)
			if manageConnection {
				// pass on information to go routine that result of association response
				cpConnectionStatus <- cpConnected
			}
		case message.MsgTypePFDManagementRequest:
			outgoingMessage = pconn.handlePFDMgmtRequest(upf, msg, pPkt.Address, sourceIP)
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = pconn.handleSessionEstablishmentRequest(upf, msg, pPkt.Address, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = pconn.handleSessionModificationRequest(upf, msg, pPkt.Address, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, pPkt.Address, upf.recoveryTime)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = pconn.handleSessionDeletionRequest(upf, msg, pPkt.Address, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(msg, pPkt.Address, sourceIP, accessIP, upf.recoveryTime)
			cleanupSessions()
		default:
			log.Println("Message type: ", msg.MessageTypeName(), " is currently not supported")
			return
		}

		// send the response out
		if outgoingMessage != nil {
			if _, err := conn.WriteTo(outgoingMessage, pPkt.Address); err != nil {
				log.Fatalln("Unable to transmit association setup response", err)
			}
		}
	}
	go pfcpPacketReader(pfcpRcvdPktsChan)
	for {
		select {
		// create goroutine for each packet parsing
		case rPkt := <-pfcpRcvdPktsChan:
			go pfcpPacketParsing(rPkt, pfcpParsedPktsChan)

		// only 1 function to process packet.
		// assumption is that Parsing is heavy and
		// packet processing is light
		case pPkt := <-pfcpParsedPktsChan:
			pfcpPacketProcessing(pPkt)

		}
	}
}
