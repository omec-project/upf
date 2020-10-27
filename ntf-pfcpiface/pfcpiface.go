// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// PktBufSz : buffer size for incoming pkt
const (
	PktBufSz    = 1500
	PFCPPort    = "8806"
	MaxItems    = 10
	Timeout     = 1000 * time.Millisecond
	readTimeout = 25 * time.Second
)

type sessRecord struct {
	pdrs []pdr
	fars []far
}

var sessions map[uint64]sessRecord

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

var seqNum sequenceNumber

func pfcpifaceMainLoop(upf *upf, accessIP, coreIP, sourceIP, smfName string) {
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
	//cpConnected is true if upf received request from control plane or
	// if upf receives +ve response for upf initiated setup request
	cpConnected := false

	// cleanup the pipeline
	cleanupSessions := func() {
		if cpConnected {
			sendDeleteAllSessionsMsgtoUPF(upf)
			cpConnected = false
		}
	}
	//initiate connection if smf address available
	log.Println("calling manageSmfConnection smf service name ", smfName)
	manageConnection := false
	if smfName != "" {
		manageConnection = true
		go manageSmfConnection(sourceIP, accessIP, smfName, conn, cpConnectionStatus)
	}

	// Initialize pkt buf
	buf := make([]byte, PktBufSz)
	// Initialize pkt header

	// Initialize sessions map
	sessions = make(map[uint64]sessRecord)

	for {
		err := conn.SetReadDeadline(time.Now().Add(readTimeout))
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

		//log.Println("Message: ", msg)

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			outgoingMessage = handleAssociationSetupRequest(msg, addr, sourceIP, accessIP, coreIP)
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
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = handleSessionEstablishmentRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(msg, addr, sourceIP, accessIP)
			cleanupSessions()
		case message.MsgTypePFDManagementRequest:
			outgoingMessage = handlePFDManagementRequest(msg, addr)
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

func handlePFDManagementRequest(msg message.Message, addr net.Addr) []byte {
	pfdreq, ok := msg.(*message.PFDManagementRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	appId, err := pfdreq.ApplicationIDsPFDs[0].ApplicationID()
	if err != nil {
		log.Fatalln("Can't read application ID")
	}
	log.Println("Got PFD for application ID: ", appId)

	log.Println(pfdreq.ApplicationIDsPFDs[0])
	pfdContext := pfdreq.ApplicationIDsPFDs[0].PFDContext()

	for ctx := range pfdContext {
		log.Println(ctx)
	}

	pfdContents, err := pfdContext[0].PFDContents()
	if err != nil {
		log.Fatalln("I have no idea what I am doing")
	}

	log.Println("Found contents: ", pfdContents)

	pfdres, err := message.NewPFDManagementResponse(pfdreq.SequenceNumber,
		ie.NewCause(ie.CauseRequestRejected),
		ie.NewOffendingIE(ie.PFDContents),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create PFD management response to: ", addr)
	}

	log.Println("Sent PFD management response to: ", addr)
	return pfdres
}

func handleAssociationSetupRequest(msg message.Message, addr net.Addr, sourceIP string, accessIP string, coreIP string) []byte {
	asreq, ok := msg.(*message.AssociationSetupRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	ts, err := asreq.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		log.Println("Got an association setup request with invalid TS: ", err, " from: ", addr)
		return nil
	}
	log.Println("Got an association setup request with TS: ", ts, " from: ", addr)

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	asres, err := message.NewAssociationSetupResponse(asreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, accessIP, "", "", ie.SrcInterfaceAccess),
		//ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association setup response", err)
	}

	log.Println("Sent association setup response to: ", addr)

	return asres
}

func handleAssociationSetupResponse(msg message.Message, addr net.Addr, sourceIP string, accessIP string) bool {
	asres, ok := msg.(*message.AssociationSetupResponse)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return false
	}

	ts, err := asres.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		log.Println("Got an association setup response with invalid TS: ", err, " from: ", addr)
		return false
	}
	log.Println("Received a PFCP association setup response with TS: ", ts, " from: ", addr)

	cause, err := asres.Cause.Cause()
	if err != nil {
		log.Println("Got an association setup response without casue ", err, " from: ", addr, "Cause ", cause)
		return false
	}

	log.Println("PFCP Association formed with Control Plane - ", addr)
	return true
}

func handleAssociationReleaseRequest(msg message.Message, addr net.Addr, sourceIP string, accessIP string) []byte {
	arreq, ok := msg.(*message.AssociationReleaseRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got an association release request from: ", addr)

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	arres, err := message.NewAssociationReleaseResponse(arreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, accessIP, "", "", ie.SrcInterfaceAccess),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association release response", err)
	}

	log.Println("Sent association release response to: ", addr)

	return arres
}

func handleSessionEstablishmentRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)

	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		log.Println("Failed to parse FSEID from session establishment request")
		return nil
	}

	/* Read CreatePDRs and CreateFARs from payload */
	pdrs, fars, err := parsePDRsFARs(upf, sereq, fseid)
	if err != nil {
		log.Println(err)
		// Build response message
		seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
			0,                              /* FO <-- what's this? */
			fseid.SEID,                     /* seid */
			sereq.SequenceNumber,           /* seq # */
			0,                              /* priority */
			ie.NewNodeID(sourceIP, "", ""), /* node id (IPv4) */
			ie.NewCause(ie.CauseRequestRejected),
		).Marshal()

		if err != nil {
			log.Fatalln("Unable to create session establishment response", err)
		}

		log.Println("Sending session establishment response to: ", addr)
		return seres
	}

	upf.sendMsgToUPF("add", pdrs, fars)

	// Adding current session details to the hash map
	sessItem := sessRecord{
		pdrs: pdrs,
		fars: fars,
	}
	sessions[fseid.SEID] = sessItem

	// Build response message
	seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		fseid.SEID,                           /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(peerSEID(fseid.SEID), net.ParseIP(sourceIP), nil, nil),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session establishment response", err)
	}

	log.Println("Sending session establishment response to: ", addr)

	return seres
}

func handleSessionModificationRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session modification request from: ", addr)

	/* fetch FSEID */
	fseid := (mySEID(smreq.SEID()))

	/* initialize farList */
	fars := make([]far, 0, MaxItems)

	/* read FAR(s). These can be multiple */
	ies1, err := ie.ParseMultiIEs(smreq.Payload)
	if err != nil {
		log.Println("Failed to parse smreq for IEs!")
	} else {
		/*
		 * Iteratively go through all IEs. You can't use ie.UpdateFAR since a single
		 * message can carry multiple UpdateFAR messages.
		 */
		for _, ie1 := range ies1 {
			switch ie1.Type {
			case ie.UpdateFAR:
				if f := parseUpdateFAR(ie1, fseid, upf.accessIP); f != nil {
					fars = append(fars, *f)
				}
			default:
				/* more will be added later */
			}
		}
	}

	upf.sendMsgToUPF("add", nil, fars)

	// Build response message
	smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		(mySEID(smreq.SEID())),               /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(peerSEID(smreq.SEID()), net.ParseIP(sourceIP), nil, nil),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session modification response", err)
	}

	log.Println("Sent session modification response to: ", addr)

	return smres
}

func handleHeartbeatRequest(msg message.Message, addr net.Addr) []byte {
	hbreq, ok := msg.(*message.HeartbeatRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a heartbeat request from: ", addr)

	// Build response message
	hbres, err := message.NewHeartbeatResponse(hbreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(time.Now()), /* ts */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create heartbeat response", err)
	}

	log.Println("Sent heartbeat response to: ", addr)

	return hbres
}

func handleSessionDeletionRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)

	/* retrieve sessionRecord */
	sessItem := sessions[mySEID(sdreq.SEID())]

	upf.sendMsgToUPF("del", sessItem.pdrs, sessItem.fars)

	/* delete sessionRecord */
	delete(sessions, mySEID(sdreq.SEID()))

	// Build response message
	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		mySEID(sdreq.SEID()),                 /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return smres
}

func getSeqNum() uint32 {
	seqNum.mux.Lock()
	defer seqNum.mux.Unlock()
	seqNum.seq++
	return seqNum.seq
}

func manageSmfConnection(n4LocalIP string, n3ip string, n4Dst string, conn *net.UDPConn, cpConnectionStatus chan bool) {
	msg := false
	cpConnected := false

	initiatePfcpConnection := func() {
		log.Println("SPGWC/SMF hostname ", n4Dst)
		n4DstIP := getRemoteIP(n4Dst)
		log.Println("SPGWC/SMF address IP inside manageSmfConnection ", n4DstIP.String())
		// initiate request if we have control plane address available
		if n4DstIP.String() != "0.0.0.0" {
			generateAssociationRequest(n4LocalIP, n3ip, n4DstIP.String(), conn)
		}
		// no worry. Looks like control plane is still not up
	}
	updateSmfStatus := func(msg bool) {
		log.Println("cpConnected : ", cpConnected, "msg ", msg)
		//events from main Loop
		if cpConnected && !msg {
			log.Println("CP disconnected ")
			cpConnected = false
		} else if !cpConnected && msg {
			log.Println("CP Connected ")
			cpConnected = true
		} else {
			log.Println("cpConnected ", cpConnected, "msg - ", msg)
		}
	}

	initiatePfcpConnection()

	connHelathTicker := time.NewTicker(5000 * time.Millisecond)
	pfcpResponseTicker := time.NewTicker(2000 * time.Millisecond)
	for {
		select {
		case msg = <-cpConnectionStatus:
			//events from main Loop
			updateSmfStatus(msg)
			if cpConnected {
				pfcpResponseTicker.Stop()
			}
		case <-connHelathTicker.C:
			if !cpConnected {
				log.Println("Retry pfcp connection setup ", n4Dst)
				initiatePfcpConnection()
			}
		case <-pfcpResponseTicker.C:
			log.Println("PFCP session setup timeout ")
			pfcpResponseTicker.Stop()
			// we will attempt new connection after next recheck
		}
	}
}

func generateAssociationRequest(n4LocalIP string, n3ip string, n4DstIp string, conn *net.UDPConn) {

	seq_num := getSeqNum()
	log.Println("n4DstIp ", n4DstIp)
	// Build request message
	asreq, err := message.NewAssociationSetupRequest(seq_num, ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(n4LocalIP, "", ""), /* node id (IPv4) */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, n3ip, "", "", ie.SrcInterfaceAccess),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association setup response", err)
	}

	smfAddr, err := net.ResolveUDPAddr("udp", n4DstIp+":"+PFCPPort)
	if err != nil {
		log.Fatalln("Unable to resolve udp addr!", err)
		return
	}

	log.Println("SMF address ", smfAddr)

	if _, err := conn.WriteTo(asreq, smfAddr); err != nil {
		log.Fatalln("Unable to transmit association setup request ", err)
	}
}
