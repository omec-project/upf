// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func handleHeartbeatRequest(msg message.Message, addr net.Addr, rTime time.Time) []byte {
	hbreq, ok := msg.(*message.HeartbeatRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a heartbeat request from: ", addr)

	// Build response message
	hbres, err := message.NewHeartbeatResponse(hbreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(rTime), /* ts */
	).Marshal()
	if err != nil {
		log.Fatalln("Unable to create heartbeat response", err)
	}

	log.Println("Sent heartbeat response to: ", addr)

	return hbres
}

func (pc *PFCPConn) handleAssociationSetupRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string, accessIP string, coreIP string) []byte {
	asreq, ok := msg.(*message.AssociationSetupRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	nodeID, err := asreq.NodeID.NodeID()
	if err != nil {
		log.Println("Got an association setup request with invalid NodeID: ", err, " from: ", addr)
		return nil
	}
	ts, err := asreq.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		log.Println("Got an association setup request with invalid TS: ", err, " from: ", addr)
		return nil
	}
	log.Println("Got an association setup request with TS: ", ts, " from: ", addr)
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Association_Setup_Request", "Incoming", "Success").Inc()

	cause := ie.CauseRequestAccepted
	if !upf.isConnected() {
		cause = ie.CauseRequestRejected
		globalPfcpStats.messages.WithLabelValues(string(nodeID), "Association_Setup_Response", "Outgoing", "Failure").Inc()
	} else {
		globalPfcpStats.messages.WithLabelValues(string(nodeID), "Association_Setup_Response", "Outgoing", "Success").Inc()
	}

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	flags := uint8(0x41)
	log.Println("Dnn info : ", upf.dnn)
	if len(upf.dnn) != 0 {
		//add ASSONI flag to set network instance.
		flags = uint8(0x61)
	}
	asresmsg := message.NewAssociationSetupResponse(asreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(upf.recoveryTime),
		ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
		ie.NewCause(cause),                        /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(flags, 0, upf.accessIP.String(), "", upf.dnn, ie.SrcInterfaceAccess),
		// ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
	) /* userplane ip resource info */

	pc.mgr.nodeID = nodeID
	log.Println("Association setup NodeID : ", pc.mgr.nodeID)
	features := make([]uint8, 4)
	if upf.enableUeIPAlloc {
		setUeipFeature(features...)
	}

	if upf.enableEndMarker {
		setEndMarkerFeature(features...)
	}

	asresmsg.UPFunctionFeatures =
		ie.NewUPFunctionFeatures(features...)
	asres, err := asresmsg.Marshal()
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

func handleAssociationReleaseRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string, accessIP string, rTime time.Time) []byte {
	arreq, ok := msg.(*message.AssociationReleaseRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got an association release request from: ", addr)

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	arres, err := message.NewAssociationReleaseResponse(arreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(rTime),
		ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted),      /* accept it blindly for the time being */
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

func (pc *PFCPConn) handlePFDMgmtRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	pfdmreq, ok := msg.(*message.PFDManagementRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a PFD management request from: ", addr)

	currentAppPFDs := pc.mgr.appPFDs

	// On every PFD management request reset existing contents
	// TODO: Analyse impact on PDRs referencing these IDs
	pc.mgr.ResetAppPFDs()

	sendError := func(err error, offendingIE *ie.IE) []byte {
		// Revert the map to original contents
		pc.mgr.appPFDs = currentAppPFDs
		log.Println(err)
		// Build response message
		pfdres, err := message.NewPFDManagementResponse(pfdmreq.SequenceNumber,
			ie.NewCause(ie.CauseRequestRejected),
			offendingIE,
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create PFD management response", err)
		}

		log.Println("Sending PFD management error response to: ", addr)
		return pfdres
	}

	for _, appIDPFD := range pfdmreq.ApplicationIDsPFDs {
		id, err := appIDPFD.ApplicationID()
		if err != nil {
			return sendError(err, appIDPFD)
		}

		pc.mgr.NewAppPFD(id)
		appPFD := pc.mgr.appPFDs[id]

		pfdCtx, err := appIDPFD.PFDContext()
		if err != nil {
			pc.mgr.RemoveAppPFD(id)
			return sendError(err, appIDPFD)
		}

		for _, pfdContent := range pfdCtx {
			fields, err := pfdContent.PFDContents()
			if err != nil {
				pc.mgr.RemoveAppPFD(id)
				return sendError(err, appIDPFD)
			}
			if fields.FlowDescription == "" {
				return sendError(errors.New("Flow Description not found"), appIDPFD)
			}
			appPFD.flowDescs = append(appPFD.flowDescs, fields.FlowDescription)
		}
		pc.mgr.appPFDs[id] = appPFD
		log.Println("Flow descriptions for AppID", id, ":", appPFD.flowDescs)
	}

	// Build response message
	pfdres, err := message.NewPFDManagementResponse(pfdmreq.SequenceNumber,
		ie.NewCause(ie.CauseRequestAccepted),
		nil,
	).Marshal()
	if err != nil {
		log.Fatalln("Unable to create PFD management response", err)
	}

	log.Println("Sending PFD management response to: ", addr)
	return pfdres
}

func (pc *PFCPConn) handleSessionReportResponse(upf *upf, msg message.Message, addr net.Addr) {
	log.Println("Got session report response from: ", addr)
	srres, ok := msg.(*message.SessionReportResponse)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return
	}

	cause := srres.Cause.Payload[0]
	if cause != ie.CauseRequestAccepted {
		seid := srres.SEID()
		log.Println("session req not accepted seq : ", srres.SequenceNumber)
		if cause == ie.CauseSessionContextNotFound {
			sessItem, ok := pc.mgr.sessions[seid]
			if !ok {
				log.Println("context not found locally or remote. SEID : ", seid)
				return
			}
			log.Println("context not found. Delete session locally")
			pc.mgr.RemoveSession(srres.SEID())
			cause := upf.sendMsgToUPF("del", sessItem.pdrs, sessItem.fars, sessItem.qers)
			if cause == ie.CauseRequestRejected {
				log.Println("Write to FastPath failed")
			}

			return
		}
	}

	log.Println("session req accepted seq : ", srres.SequenceNumber)
}

func (pc *PFCPConn) handleSessionEstablishmentRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	nodeID, err := sereq.NodeID.NodeID()
	if err != nil {
		log.Println("Failed to parse NodeID from session establishment request")
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Establishment_Request", "Incoming", "Success").Inc()

	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		log.Println("Failed to parse FSEID from session establishment request")
		return nil
	}
	remoteSEID := fseid.SEID
	fseidIP := ip2int(fseid.IPv4Address)

	sendError := func(err error, cause uint8) []byte {
		globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Establishment_Response", "Outgoing", "Failure").Inc()
		log.Println(err)
		// Build response message
		seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
			0,                    /* FO <-- what's this? */
			remoteSEID,           /* seid */
			sereq.SequenceNumber, /* seq # */
			0,                    /* priority */
			ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
			ie.NewCause(cause),
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create session establishment response", err)
		}

		log.Println("Sending session establishment response to: ", addr)
		return seres
	}

	if strings.Compare(nodeID, pc.mgr.nodeID) != 0 {
		log.Println("Association not found for Establishment request, nodeID: ", nodeID, ", Association NodeID: ", pc.mgr.nodeID)
		return sendError(errors.New("No Association found for NodeID"),
			ie.CauseNoEstablishedPFCPAssociation)
	}

	/* Read CreatePDRs and CreateFARs from payload */
	localSEID := pc.mgr.NewPFCPSession(remoteSEID)
	if localSEID == 0 {
		sendError(errors.New("Unable to allocate new PFCP session"),
			ie.CauseNoResourcesAvailable)
	}
	session := pc.mgr.sessions[localSEID]
	for _, cPDR := range sereq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, session.localSEID, pc.mgr.appPFDs, upf); err != nil {
			return sendError(err, ie.CauseRequestRejected)
		}
		p.fseidIP = fseidIP
		session.CreatePDR(p)
	}

	for _, cFAR := range sereq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, session.localSEID, upf, create); err != nil {
			return sendError(err, ie.CauseRequestRejected)
		}
		f.fseidIP = fseidIP
		session.CreateFAR(f)
	}

	for _, cQER := range sereq.CreateQER {
		var q qer
		if err := q.parseQER(cQER, session.localSEID, upf); err != nil {
			return sendError(err, ie.CauseRequestRejected)
		}
		q.fseidIP = fseidIP
		session.CreateQER(q)
	}

	cause := upf.sendMsgToUPF("add", session.pdrs, session.fars, session.qers)
	if cause == ie.CauseRequestRejected {
		pc.mgr.RemoveSession(session.localSEID)
		return sendError(errors.New("Write to FastPath failed"),
			ie.CauseRequestRejected)
	}

	// Build response message
	seresMsg := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                    /* FO <-- what's this? */
		session.remoteSEID,   /* seid */
		sereq.SequenceNumber, /* seq # */
		0,                    /* priority */
		ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted),      /* accept it blindly for the time being */
		ie.NewFSEID(session.localSEID, net.ParseIP(sourceIP), nil, nil),
	)

	addPdrInfo(seresMsg, session)
	seres, err := seresMsg.Marshal()
	if err != nil {
		log.Fatalln("Unable to create session establishment response", err)
	}

	log.Println("Sending session establishment response to: ", addr)
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Establishment_Response", "Outgoing", "Success").Inc()
	return seres
}

func (pc *PFCPConn) handleSessionModificationRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	nodeID := pc.mgr.nodeID

	log.Println("Got a session modification request from: ", addr)
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Modification_Request", "Incoming", "Success").Inc()

	var remoteSEID uint64
	sendError := func(err error) []byte {
		globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Modification_Response", "Outgoing", "Failure").Inc()
		log.Println(err)
		smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			remoteSEID,                           /* seid */
			smreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create session modification response", err)
		}

		log.Println("Sending session modification response to: ", addr)
		return smres
	}

	localSEID := smreq.SEID()
	session, ok := pc.mgr.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("Session not found: %v", localSEID))
	}

	var fseidIP uint32
	if smreq.CPFSEID != nil {
		fseid, err := smreq.CPFSEID.FSEID()
		if err == nil {
			session.remoteSEID = fseid.SEID
			fseidIP = ip2int(fseid.IPv4Address)
			log.Println("Updated FSEID from session modification request")
		}
	}
	remoteSEID = session.remoteSEID

	addPDRs := make([]pdr, 0, MaxItems)
	addFARs := make([]far, 0, MaxItems)
	addQERs := make([]qer, 0, MaxItems)
	endMarkerList := make([][]byte, 0, MaxItems)
	for _, cPDR := range smreq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, localSEID, pc.mgr.appPFDs, upf); err != nil {
			return sendError(err)
		}
		p.fseidIP = fseidIP
		session.CreatePDR(p)
		addPDRs = append(addPDRs, p)
	}

	for _, cFAR := range smreq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, localSEID, upf, create); err != nil {
			return sendError(err)
		}
		f.fseidIP = fseidIP
		session.CreateFAR(f)
		addFARs = append(addFARs, f)
	}

	for _, cQER := range smreq.CreateQER {
		var q qer
		if err := q.parseQER(cQER, localSEID, upf); err != nil {
			return sendError(err)
		}
		q.fseidIP = fseidIP
		session.CreateQER(q)
		addQERs = append(addQERs, q)
	}

	for _, uPDR := range smreq.UpdatePDR {
		var p pdr
		var err error
		if err = p.parsePDR(uPDR, localSEID, pc.mgr.appPFDs, upf); err != nil {
			return sendError(err)
		}
		p.fseidIP = fseidIP
		err = session.UpdatePDR(p)
		if err != nil {
			log.Println("session PDR update failed ", err)
			continue
		}
		addPDRs = append(addPDRs, p)
	}

	for _, uFAR := range smreq.UpdateFAR {
		var f far
		var err error
		if err = f.parseFAR(uFAR, localSEID, upf, update); err != nil {
			return sendError(err)
		}
		f.fseidIP = fseidIP
		err = session.UpdateFAR(&f, &endMarkerList)
		if err != nil {
			log.Println("session PDR update failed ", err)
			continue
		}
		addFARs = append(addFARs, f)
	}

	for _, uQER := range smreq.UpdateQER {
		var q qer
		var err error
		if err = q.parseQER(uQER, localSEID, upf); err != nil {
			return sendError(err)
		}
		q.fseidIP = fseidIP
		err = session.UpdateQER(q)
		if err != nil {
			log.Println("session QER update failed ", err)
			continue
		}
		addQERs = append(addQERs, q)
	}

	if session.getNotifyFlag() {
		session.updateNotifyFlag()
	}

	cause := upf.sendMsgToUPF("mod", addPDRs, addFARs, addQERs)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("Write to FastPath failed"))
	}

	if upf.enableEndMarker {
		err := upf.sendEndMarkers(&endMarkerList)
		if err != nil {
			log.Println("Sending End Markers Failed : ", err)
		}
	}

	delPDRs := make([]pdr, 0, MaxItems)
	delFARs := make([]far, 0, MaxItems)
	delQERs := make([]qer, 0, MaxItems)

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

		f, err := session.RemoveFAR(farID)
		if err != nil {
			return sendError(err)
		}
		delFARs = append(delFARs, *f)
	}

	for _, dQER := range smreq.RemoveQER {
		qerID, err := dQER.QERID()
		if err != nil {
			return sendError(err)
		}

		q, err := session.RemoveQER(qerID)
		if err != nil {
			return sendError(err)
		}
		delQERs = append(delQERs, *q)
	}

	cause = upf.sendMsgToUPF("del", delPDRs, delFARs, delQERs)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("Write to FastPath failed"))
	}

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

	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Modification_Response", "Outgoing", "Success").Inc()
	log.Println("Sent session modification response to: ", addr)
	return smres
}

func (pc *PFCPConn) handleSessionDeletionRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	nodeID := pc.mgr.nodeID
	log.Println("Got a session deletion request from: ", addr)
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Deletion_Request", "Incoming", "Success").Inc()

	sendError := func(err error) []byte {
		log.Println(err)
		globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Deletion_Response", "Outgoing", "Failure").Inc()
		smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			0,                                    /* seid */
			sdreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		).Marshal()
		if err != nil {
			log.Fatalln("Unable to create session deletion response", err)
		}

		log.Println("Sending session deletion response to: ", addr)
		return smres
	}

	/* retrieve sessionRecord */
	localSEID := sdreq.SEID()
	session, ok := pc.mgr.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("Session not found: %v", localSEID))
	}

	cause := upf.sendMsgToUPF("del", session.pdrs, session.fars, session.qers)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("Write to FastPath failed"))
	}

	releaseAllocatedIPs(upf, session)
	/* delete sessionRecord */
	pc.mgr.RemoveSession(localSEID)

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
	globalPfcpStats.messages.WithLabelValues(string(nodeID), "Pfcp_Deletion_Response", "Outgoing", "Success").Inc()

	return smres
}

func (pc *PFCPConn) manageSmfConnection(n4LocalIP string, n3ip string, n4Dst string, conn *net.UDPConn, cpConnectionStatus chan bool, rTime time.Time) {
	cpConnected := false

	initiatePfcpConnection := func() {
		log.Println("SPGWC/SMF hostname ", n4Dst)
		n4DstIP := getRemoteIP(n4Dst)
		log.Println("SPGWC/SMF address IP inside manageSmfConnection ", n4DstIP.String())
		// initiate request if we have control plane address available
		if n4DstIP.String() != "0.0.0.0" {
			pc.generateAssociationRequest(n4LocalIP, n3ip, n4DstIP.String(), conn, rTime)
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

func (pc *PFCPConn) generateAssociationRequest(n4LocalIP string, n3ip string, n4DstIP string, conn *net.UDPConn, rTime time.Time) {
	seq := pc.getSeqNum()
	log.Println("n4DstIp ", n4DstIP)
	// Build request message
	asreq, err := message.NewAssociationSetupRequest(seq, ie.NewRecoveryTimeStamp(rTime),
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

func readReportNotification(rn <-chan uint64, pfcpConn *PFCPConn,
	udpConn *net.UDPConn, udpAddr net.Addr) {
	log.Println("read report notification start")
	for {
		select {
		case fseid := <-rn:
			handleDigestReport(fseid, pfcpConn, udpConn, udpAddr)

		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func handleDigestReport(fseid uint64,
	pfcpConn *PFCPConn,
	udpConn *net.UDPConn,
	udpAddr net.Addr) {
	session, ok := pfcpConn.mgr.sessions[fseid]
	if !ok {
		log.Println("No session found for fseid : ", fseid)
		return
	}

	/* Number of Outstanding Notifies per session is 1 */
	if session.getNotifyFlag() {
		return
	}

	session.setNotifyFlag(true)
	seq := pfcpConn.getSeqNum()
	serep := message.NewSessionReportRequest(0, /* MO?? <-- what's this */
		0,                            /* FO <-- what's this? */
		0,                            /* seid */
		seq,                          /* seq # */
		0,                            /* priority */
		ie.NewReportType(0, 0, 0, 1), /*upir, erir, usar, dldr int*/
	)
	serep.Header.SEID = session.remoteSEID
	var pdrID uint32
	for _, pdr := range session.pdrs {
		if pdr.srcIface == core {
			pdrID = pdr.pdrID
			break
		}
	}

	log.Println("Pdr iD : ", pdrID)
	if pdrID == 0 {
		log.Println("No Pdr found for downlink")
		return
	}

	serep.DownlinkDataReport = ie.NewDownlinkDataReport(
		ie.NewPDRID(uint16(pdrID)))

	ret, err := serep.Marshal()
	if err != nil {
		log.Println("Marshal function failed for SM resp ", err)
	}

	// send the report req out
	if ret != nil {
		if _, err := udpConn.WriteTo(ret, udpAddr); err != nil {
			log.Fatalln("Unable to transmit Report req", err)
		}
	}
}
