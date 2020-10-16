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
	PFCPPort    = "8805"
	MaxItems    = 10
	Timeout     = 1000 * time.Millisecond
	readTimeout = 25 * time.Second
)

type sessRecord struct {
	pdrs []pdr
	fars []far
	urrs []urr
}

var sessions map[uint64]sessRecord

type sequenceNumber struct {
	seq uint32
	mux sync.Mutex
}

var seqNum sequenceNumber

type reportRecord struct {
	fseid uint64
	srcIP string
	urrs  *[]urr
}

func (r *reportRecord) checkInvalid() bool {
	var flag bool = true
	for _, urr := range *r.urrs {
		if urr.reportOpen {
			flag = false
			break
		}
	}
	return flag
}

func pfcpifaceMainLoop(intf common, accessIP, coreIP, sourceIP, smfName string) {
	upfPt := intf.getUpf()
	log.Println("pfcpifaceMainLoop@" + upfPt.fqdnHost + " says hello!!!")
	log.Println("n4 ip ", sourceIP)
	log.Println("access ip ", accessIP)
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
			intf.sendDeleteAllSessionsMsgtoUPF()
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
	log.Println("After allocating session record")
	PrintMemUsage()
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
			intf.sendDeleteAllSessionsMsgtoUPF()
			intf.setUdpConn(conn, addr)
			outgoingMessage = handleAssociationSetupRequest(intf, msg, addr, sourceIP, accessIP, coreIP)
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
			outgoingMessage = handleSessionEstablishmentRequest(intf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(intf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(intf, msg, addr, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(msg, addr, sourceIP, accessIP)
			cleanupSessions()
		case message.MsgTypeSessionReportResponse:
			handleSessionReportResponse(msg, addr, intf)
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

func handleAssociationSetupRequest(intf common, msg message.Message, addr net.Addr, sourceIP string, accessIP string, coreIP string) []byte {
	asreq, ok := msg.(*message.AssociationSetupRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	fail := intf.handleChannelStatus()

	var cause uint8
	if fail {
		cause = ie.CauseRequestRejected
	} else {
		cause = ie.CauseRequestAccepted
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
		ie.NewNodeID(sourceIP, "", ""), /* node id (IPv4) */
		ie.NewCause(cause),             /* accept it blindly for the time being */
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

func handleSessionReportResponse(msg message.Message, addr net.Addr, intf common) {
	srres, ok := msg.(*message.SessionReportResponse)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return
	}

	cause := srres.Cause.Payload[0]
	if cause == ie.CauseRequestAccepted {
		log.Println("session req accepted seq : ", srres.SequenceNumber)
	} else {
		log.Println("session req not accepted seq : ", srres.SequenceNumber)
		if cause == ie.CauseSessionContextNotFound {
			log.Println("context not found. Delete session locally")
			sessItem := sessions[mySEID(srres.SEID())]

			var flag bool = intf.handleChannelStatus()

			if !flag {
				intf.sendMsgToUPF("del", sessItem.pdrs,
					sessItem.fars, sessItem.urrs)

				/* delete sessionRecord */
				delete(sessions, mySEID(srres.SEID()))
			}
		}
	}

	log.Println("Got session release response from: ", addr)
}

func handleSessionEstablishmentRequest(intf common, msg message.Message, addr net.Addr, sourceIP string) []byte {
	upfPt := intf.getUpf()
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)
	PrintMemUsage()

	var fail bool
	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		log.Println("Failed to parse FSEID from session establishment request")
		return nil
	}

	fail = intf.handleChannelStatus()

	var cause uint8
	if fail {
		cause = ie.CauseRequestRejected
	} else {
		/* Read CreatePDRs and CreateFARs from payload */
		pdrs, fars, urrs, err := parsePDRsFARs(upfPt, sereq, fseid)
		if err != nil {
			cause = ie.CauseRequestRejected
		} else {
			cause = intf.sendMsgToUPF("add", pdrs, fars, urrs)
		}

		log.Println("Before sending URR for reporting.")
		PrintMemUsage()
		if cause == ie.CauseRequestAccepted {
			// Adding current session details to the hash map
			recItem := reportRecord{
				fseid: fseid.SEID,
				srcIP: sourceIP,
				urrs:  &urrs,
			}
			intf.sendURRForReporting(&recItem)
			sessItem := sessRecord{
				pdrs: pdrs,
				fars: fars,
				urrs: urrs,
			}
			sessions[fseid.SEID] = sessItem
			cause = ie.CauseRequestAccepted
		}
		log.Println("After sending URR for reporting.")
		PrintMemUsage()
	}

	// Build response message
	seres := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                              /* FO <-- what's this? */
		fseid.SEID,                     /* seid */
		sereq.SequenceNumber,           /* seq # */
		0,                              /* priority */
		ie.NewNodeID(sourceIP, "", ""), /* node id */
		ie.NewCause(cause),             /* accept it blindly for the time being */
	)

	if cause == ie.CauseRequestAccepted {
		seres.UPFSEID = ie.NewFSEID(peerSEID(fseid.SEID),
			net.ParseIP(sourceIP), nil, nil)
	}

	ret, err := seres.Marshal()
	if err != nil {
		log.Println("Marshal function failed for SE Resp ", err)
		return nil
	}

	log.Println("Sending session establishment response to: ", addr)
	return ret
}

func handleSessionModificationRequest(intf common, msg message.Message, addr net.Addr, sourceIP string) []byte {
	//upfPt := intf.getUpf()
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

	var flag bool = intf.handleChannelStatus()

	var cause uint8 = ie.CauseRequestAccepted
	if flag {
		cause = ie.CauseRequestRejected
	} else {
		/* read FAR(s). These can be multiple */
		ies1, err := ie.ParseMultiIEs(smreq.Payload)
		if err != nil {
			log.Println("Failed to parse smreq for IEs!")
			cause = ie.CauseRequestRejected
		} else {
			/*
			 * Iteratively go through all IEs. You can't use ie.UpdateFAR since a single
			 * message can carry multiple UpdateFAR messages.
			 */
			for _, ie1 := range ies1 {
				switch ie1.Type {
				case ie.UpdateFAR:
					if f := parseUpdateFAR(ie1, fseid,
						intf.getAccessIP()); f != nil {
						fars = append(fars, *f)
					} else {
						log.Println("Parse FAR failed.")
						cause = ie.CauseRequestRejected
						break
					}
				default:
					/* more will be added later */
				}
			}
		}

		if cause == ie.CauseRequestAccepted {
			cause = intf.sendMsgToUPF("mod", nil, fars, nil)
		}
	}
	// Build response message
	smres := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                      /* FO <-- what's this? */
		(mySEID(smreq.SEID())), /* seid */
		smreq.SequenceNumber,   /* seq # */
		0,                      /* priority */
		ie.NewCause(cause),     /* accept it blindly for the time being */
	)

	ret, err := smres.Marshal()
	if err != nil {
		log.Println("Marshal function failed for SM resp ", err)
		return nil
	}

	log.Println("Sent session modification response to: ", addr)
	return ret
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

func handleSessionDeletionRequest(intf common, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)
	PrintMemUsage()

	/* retrieve sessionRecord */
	sessItem := sessions[mySEID(sdreq.SEID())]

	var flag bool = intf.handleChannelStatus()

	var cause uint8 = ie.CauseRequestAccepted
	sdRes := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                    /* FO <-- what's this? */
		mySEID(sdreq.SEID()), /* seid */
		sdreq.SequenceNumber, /* seq # */
		0,                    /* priority */
		ie.NewCause(cause),   /* accept it blindly for the time being */
	)
	if !flag {
		intf.sendMsgToUPF("del", sessItem.pdrs,
			sessItem.fars, sessItem.urrs)

		intf.addUsageReports(sdRes, mySEID(sdreq.SEID()))
		/* delete sessionRecord */
		delete(sessions, mySEID(sdreq.SEID()))
	}

	log.Println("After Deleting session.")
	PrintMemUsage()
	// Build response message
	ret, err := sdRes.Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return ret
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
