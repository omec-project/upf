// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

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

func (pc *PFCPConn) handleAssociationSetupRequest(msg message.Message, addr net.Addr, sourceIP string, accessIP string, coreIP string) []byte {
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
		// ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
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
		log.Println("Got an association setup response without cause ", err, " from: ", addr, "Cause ", cause)
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

func (pc *PFCPConn) handleSessionEstablishmentRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
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
	remoteSEID := fseid.SEID

	sendError := func(err error) []byte {
		log.Println(err)
		// Build response message
		seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
			0,                              /* FO <-- what's this? */
			remoteSEID,                     /* seid */
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

	/* Read CreatePDRs and CreateFARs from payload */
	localSEID := pc.mgr.NewPFCPSession(remoteSEID)
	if localSEID == 0 {
		sendError(errors.New("Unable to allocate new PFCP session"))
	}
	session := pc.mgr.sessions[localSEID]

	for _, cPDR := range sereq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, session.localSEID); err != nil {
			return sendError(err)
		}
		session.CreatePDR(p)
	}

	for _, cFAR := range sereq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, session.localSEID, upf, create); err != nil {
			return sendError(err)
		}
		session.CreateFAR(f)
	}

	upf.sendMsgToUPF("add", session.pdrs, session.fars)

	// Build response message
	seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(session.localSEID, net.ParseIP(sourceIP), nil, nil),
	).Marshal()
	if err != nil {
		log.Fatalln("Unable to create session establishment response", err)
	}

	log.Println("Sending session establishment response to: ", addr)
	pc.mgr.sessions[localSEID] = session
	return seres
}

func (pc *PFCPConn) handleSessionModificationRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session modification request from: ", addr)

	var remoteSEID uint64
	sendError := func(err error) []byte {
		log.Println(err)
		smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			remoteSEID,                           /* seid */
			smreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create session establishment response", err)
		}

		log.Println("Sending session establishment response to: ", addr)
		return smres
	}

	localSEID := smreq.SEID()
	session, ok := pc.mgr.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("Session not found: %v", localSEID))
	}

	if smreq.CPFSEID != nil {
		fseid, err := smreq.CPFSEID.FSEID()
		if err == nil {
			session.remoteSEID = fseid.SEID
			log.Println("Updated FSEID from session modification request")
		}
	}
	remoteSEID = session.remoteSEID

	addPDRs := make([]pdr, 0, MaxItems)
	addFARs := make([]far, 0, MaxItems)

	for _, cPDR := range smreq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, localSEID); err != nil {
			return sendError(err)
		}
		session.CreatePDR(p)
		addPDRs = append(addPDRs, p)
	}

	for _, cFAR := range smreq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, localSEID, upf, create); err != nil {
			return sendError(err)
		}
		session.CreateFAR(f)
		addFARs = append(addFARs, f)
	}

	for _, uPDR := range smreq.UpdatePDR {
		var p pdr
		if err := p.parsePDR(uPDR, localSEID); err != nil {
			return sendError(err)
		}
		session.UpdatePDR(p)
		addPDRs = append(addPDRs, p)
	}

	for _, uFAR := range smreq.UpdateFAR {
		var f far
		if err := f.parseFAR(uFAR, localSEID, upf, update); err != nil {
			return sendError(err)
		}
		session.UpdateFAR(f)
		addFARs = append(addFARs, f)
	}

	upf.sendMsgToUPF("add", addPDRs, addFARs)

	delPDRs := make([]pdr, 0, MaxItems)
	delFARs := make([]far, 0, MaxItems)

	for _, rPDR := range smreq.RemovePDR {
		pdrID, err := rPDR.PDRID()
		if err != nil {
			return sendError(err)
		}

		p, err := session.RemovePDR(uint32(pdrID))
		if err != nil {
			return sendError(err)
		}
		delPDRs = append(delPDRs, *p)
	}

	for _, dFAR := range smreq.RemoveFAR {
		farID, err := dFAR.FARID()
		if err != nil {
			return sendError(err)
		}

		f, err := session.RemoveFAR(uint8(farID))
		if err != nil {
			return sendError(err)
		}
		delFARs = append(delFARs, *f)
	}

	upf.sendMsgToUPF("del", delPDRs, delFARs)

	// Build response message
	smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		remoteSEID,                           /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	).Marshal()
	if err != nil {
		log.Fatalln("Unable to create session modification response", err)
	}

	log.Println("Sent session modification response to: ", addr)
	pc.mgr.sessions[localSEID] = session
	return smres
}

func (pc *PFCPConn) handleSessionDeletionRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)

	sendError := func(err error) []byte {
		log.Println(err)
		smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			0,                                    /* seid */
			sdreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create session establishment response", err)
		}

		log.Println("Sending session establishment response to: ", addr)
		return smres
	}

	/* retrieve sessionRecord */
	localSEID := sdreq.SEID()
	session, ok := pc.mgr.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("Session not found: %v", localSEID))
	}

	upf.sendMsgToUPF("del", session.pdrs, session.fars)

	/* delete sessionRecord */
	delete(pc.mgr.sessions, localSEID)

	// Build response message
	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
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

func (pc *PFCPConn) manageSmfConnection(n4LocalIP string, n3ip string, n4Dst string, conn *net.UDPConn, cpConnectionStatus chan bool) {
	cpConnected := false

	initiatePfcpConnection := func() {
		log.Println("SPGWC/SMF hostname ", n4Dst)
		n4DstIP := getRemoteIP(n4Dst)
		log.Println("SPGWC/SMF address IP inside manageSmfConnection ", n4DstIP.String())
		// initiate request if we have control plane address available
		if n4DstIP.String() != "0.0.0.0" {
			pc.generateAssociationRequest(n4LocalIP, n3ip, n4DstIP.String(), conn)
		}
		// no worry. Looks like control plane is still not up
	}
	updateSmfStatus := func(msg bool) {
		log.Println("cpConnected : ", cpConnected, "msg ", msg)
		// events from main Loop
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
		case msg := <-cpConnectionStatus:
			// events from main Loop
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

func (pc *PFCPConn) generateAssociationRequest(n4LocalIP string, n3ip string, n4DstIP string, conn *net.UDPConn) {
	seq := pc.getSeqNum()
	log.Println("n4DstIp ", n4DstIP)
	// Build request message
	asreq, err := message.NewAssociationSetupRequest(seq, ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(n4LocalIP, "", ""), /* node id (IPv4) */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, n3ip, "", "", ie.SrcInterfaceAccess),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association setup response", err)
	}

	smfAddr, err := net.ResolveUDPAddr("udp", n4DstIP+":"+PFCPPort)
	if err != nil {
		log.Fatalln("Unable to resolve udp addr!", err)
		return
	}

	log.Println("SMF address ", smfAddr)

	if _, err := conn.WriteTo(asreq, smfAddr); err != nil {
		log.Fatalln("Unable to transmit association setup request ", err)
	}
}
