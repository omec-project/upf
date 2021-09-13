// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"net"
	"strings"
	"sync"
	"time"

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

// PFCPConn represents a PFCP connection.
type PFCPConn struct {
	seqNum   sequenceNumber
	mgr      *PFCPSessionMgr
	hbStatus chan bool
}

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

type PfcpMessage struct {
	srcAddr    net.Addr
	numOfBytes int
	buffer     message.Message
}

type PFCPNodeInfo struct {
	sourceIP *string
	coreIP   *string
	accessIP *string
}

func (c *PFCPConn) getSeqNum() uint32 {
	c.seqNum.mux.Lock()
	defer c.seqNum.mux.Unlock()
	c.seqNum.seq++

	return c.seqNum.seq
}

// cleanup the pipeline
func cleanupSessions(upf *upf) {
	if upf.simInfo != nil {
		return
	}
	sendDeleteAllSessionsMsgtoUPF(upf)
}

func handleIncomingPfcpMsg(upf *upf, pconn *PFCPConn, conn *net.UDPConn, packet PfcpMessage, pfcpNodeIP *PFCPNodeInfo, cpConnected bool, manageConnection bool, cpConnectionStatus chan bool) (bool, bool) {
	// flag to indicate if assoc setup is received
	connReset := false
	// process packets
	var outgoingMessage []byte
	// if sourceIP is not set, fetch it from the msg header
	if *pfcpNodeIP.sourceIP == net.IPv4zero.String() {
		addrString := strings.Split(packet.srcAddr.String(), ":")
		*pfcpNodeIP.sourceIP = getLocalIP(addrString[0]).String()
		log.Println("Source IP address is now: ", *pfcpNodeIP.sourceIP)
	}

	// if nodeIP is not set, fetch it from the msg header
	if upf.nodeIP.String() == net.IPv4zero.String() {
		addrString := strings.Split(packet.srcAddr.String(), ":")
		upf.nodeIP = getLocalIP(addrString[0])
		log.Println("Node IP address is now: ", upf.nodeIP.String())
	}

	switch packet.buffer.MessageType() {
	case message.MsgTypeAssociationSetupRequest:
		cleanupSessions(upf)

		go readReportNotification(upf.reportNotifyChan, pconn, conn, packet.srcAddr)

		upf.setInfo(conn, packet.srcAddr, pconn)

		outgoingMessage = pconn.handleAssociationSetupRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP, *pfcpNodeIP.accessIP, *pfcpNodeIP.coreIP)
		if outgoingMessage != nil {
			cpConnected = true
			if manageConnection {
				// pass on information to go routine that result of association response
				cpConnectionStatus <- cpConnected
			}
			connReset = true
		}
	case message.MsgTypeAssociationSetupResponse:
		cpConnected = handleAssociationSetupResponse(packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP, *pfcpNodeIP.accessIP)

		if manageConnection {
			// pass on information to go routine that result of association response
			cpConnectionStatus <- cpConnected
		}
	case message.MsgTypePFDManagementRequest:
		outgoingMessage = pconn.handlePFDMgmtRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP)
	case message.MsgTypeSessionEstablishmentRequest:
		outgoingMessage = pconn.handleSessionEstablishmentRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP)
	case message.MsgTypeSessionModificationRequest:
		outgoingMessage = pconn.handleSessionModificationRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP)
	case message.MsgTypeHeartbeatRequest:
		outgoingMessage = handleHeartbeatRequest(packet.buffer, packet.srcAddr, upf.recoveryTime)
	case message.MsgTypeSessionDeletionRequest:
		outgoingMessage = pconn.handleSessionDeletionRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP)
	case message.MsgTypeAssociationReleaseRequest:
		outgoingMessage = handleAssociationReleaseRequest(upf, packet.buffer, packet.srcAddr, *pfcpNodeIP.sourceIP, *pfcpNodeIP.accessIP, upf.recoveryTime)
		cpConnected = false
		cleanupSessions(upf)
	case message.MsgTypeSessionReportResponse:
		pconn.handleSessionReportResponse(upf, packet.buffer, packet.srcAddr)
	case message.MsgTypeHeartbeatResponse:
		log.Println("HeartBeat response received")
		pconn.hbStatus <- true
	default:
		log.Println("Message type: ", packet.buffer.MessageTypeName(), " is currently not supported")
	}

	// send the response out
	if outgoingMessage != nil {
		if _, err := conn.WriteTo(outgoingMessage, packet.srcAddr); err != nil {
			log.Fatalln("Unable to transmit association setup response", err)
		}
	}
	return cpConnected, connReset
}

func pfcpifaceMainLoop(upf *upf, accessIP, coreIP, sourceIP, smfName string) {
	var pconn PFCPConn
	pconn.mgr = NewPFCPSessionMgr(100)
	pconn.hbStatus = make(chan bool)

	rTimeout := readTimeout
	if upf.readTimeout != 0 {
		rTimeout = upf.readTimeout
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

	// initiate connection if smf address available
	log.Println("calling manageSmfConnection smf service name ", smfName)

	manageConnection := false
	if smfName != "" {
		manageConnection = true

		go pconn.manageSmfConnection(upf.nodeIP.String(), accessIP, smfName, conn, cpConnectionStatus, upf.recoveryTime)
	}

	readChannel := make(chan PfcpMessage)
	readErr := make(chan error, 1)

	// Initialize pkt buf
	buf := make([]byte, PktBufSz)
	// Initialize pkt header

	go func() (err error) {
		defer func() { readErr <- err }()
		for {
			err := conn.SetReadDeadline(time.Now().Add(rTimeout))
			if err != nil {
				log.Printf("Unable to set deadline for read: %v\n", err)
				return err
			}
			// blocking read
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				log.Printf("Read error: %v\n", err)
				return err
			}

			// use wmnsk lib to parse the pfcp message
			msg, err := message.Parse(buf)
			if err != nil {
				log.Println("Ignoring undecodable message: ", buf, " error: ", err)
				continue
			}

			log.Traceln("Message: ", msg)

			pfcpMessage := PfcpMessage{
				srcAddr:    addr,
				numOfBytes: n,
				buffer:     msg}

			readChannel <- pfcpMessage
		}
	}()

	hbTimerRunning := false // indicates if hb timer routine is running
	var hbErrCh chan bool   // chan used by hb timer routine to send status back to pfpc main loop

	pfcpNodeIP := PFCPNodeInfo{
		sourceIP: &sourceIP,
		coreIP:   &coreIP,
		accessIP: &accessIP}

	for {
		if !hbTimerRunning {
			select {
			case err := <-readErr:
				// inform smf connection mgmt routine
				cpConnected = false

				if manageConnection {
					cpConnectionStatus <- cpConnected
				}

				cleanupSessions(upf)
				log.Fatalln("Read error:", err)
			case packet := <-readChannel:
				connSetup := false

				cpConnected, connSetup = handleIncomingPfcpMsg(upf, &pconn, conn, packet, &pfcpNodeIP, cpConnected, manageConnection, cpConnectionStatus)

				if connSetup {
					hbErrCh = pconn.handleHeartBeats(upf, conn, packet.srcAddr.(*net.UDPAddr))
					hbTimerRunning = true
				}
			}
		} else {
			select {
			case err := <-readErr:
				// inform smf connection mgmt routine
				cpConnected = false

				if manageConnection {
					cpConnectionStatus <- cpConnected
				}

				cleanupSessions(upf)
				log.Fatalln("Read error:", err)
			case packet := <-readChannel:
				connReset := false

				cpConnected, connReset = handleIncomingPfcpMsg(upf, &pconn, conn, packet, &pfcpNodeIP, cpConnected, manageConnection, cpConnectionStatus)

				if connReset {
					pconn.hbStatus <- false
					hbErrCh = pconn.handleHeartBeats(upf, conn, packet.srcAddr.(*net.UDPAddr))
				}
			case status := <-hbErrCh:
				hbTimerRunning = false
				log.Printf("Received %v", status)
				cleanupSessions(upf)
			}
		}

	}
}
