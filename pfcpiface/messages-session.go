// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2021 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

const (
	// Default timeout for DDN.
	DefaultDDNTimeout = 20
)

var errAssocNotFound = errors.New("no association found for NodeID")
var errAllocateSession = errors.New("unable to allocate new PFCP session")

func (pConn *PFCPConn) handleSessionEstablishmentRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	errUnmarshalReply := func(err error, offendingIE *ie.IE) (message.Message, error) {
		// Build response message
		pfdres := message.NewSessionEstablishmentResponse(0,
			0,
			0,
			sereq.SequenceNumber,
			0,
			ie.NewCause(ie.CauseRequestRejected),
			offendingIE,
		)

		return pfdres, errUnmarshal(err)
	}

	nodeID, err := sereq.NodeID.NodeID()
	if err != nil {
		return errUnmarshalReply(err, sereq.NodeID)
	}

	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		return errUnmarshalReply(err, sereq.CPFSEID)
	}

	remoteSEID := fseid.SEID
	fseidIP := ip2int(fseid.IPv4Address)

	errProcessReply := func(err error, cause uint8) (message.Message, error) {

		// Build response message
		seres := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
			0,                    /* FO <-- what's this? */
			remoteSEID,           /* seid */
			sereq.SequenceNumber, /* seq # */
			0,                    /* priority */
			pConn.nodeID.localIE,
			ie.NewCause(cause),
		)

		return seres, errProcess(err)
	}

	if strings.Compare(nodeID, pConn.nodeID.remote) != 0 {
		log.Warnln("Association not found for Establishment request",
			"with nodeID: ", nodeID, ", Association NodeID: ", pConn.nodeID.remote)
		return errProcessReply(errAssocNotFound, ie.CauseNoEstablishedPFCPAssociation)
	}

	/* Read CreatePDRs and CreateFARs from payload */
	localSEID := pConn.NewPFCPSession(remoteSEID)
	if localSEID == 0 {
		return errProcessReply(errAllocateSession,
			ie.CauseNoResourcesAvailable)
	}

	session := pConn.sessions[localSEID]

	for _, cPDR := range sereq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, session.localSEID, pConn.appPFDs, upf.ippool); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		p.fseidIP = fseidIP
		session.CreatePDR(p)
	}

	for _, cFAR := range sereq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, session.localSEID, upf, create); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		f.fseidIP = fseidIP
		session.CreateFAR(f)
	}

	for _, cQER := range sereq.CreateQER {
		var q qer
		if err := q.parseQER(cQER, session.localSEID, upf); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		q.fseidIP = fseidIP
		session.CreateQER(q)
	}

	session.MarkSessionQer()

	cause := upf.sendMsgToUPF(upfMsgTypeAdd, session.pdrs, session.fars, session.qers)
	if cause == ie.CauseRequestRejected {
		pConn.RemoveSession(session.localSEID)
		return errProcessReply(errors.New("write to FastPath failed"),
			ie.CauseRequestRejected)
	}

	var localFSEID *ie.IE
	localIP := pConn.LocalAddr().(*net.UDPAddr).IP
	if localIP.To4() != nil {
		localFSEID = ie.NewFSEID(session.localSEID, localIP, nil)
	} else {
		localFSEID = ie.NewFSEID(session.localSEID, nil, localIP)
	}

	// Build response message
	seres := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		pConn.nodeID.localIE,                 /* node id */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		localFSEID,
	)

	addPdrInfo(seres, session)

	return seres, nil
}

func (pConn *PFCPConn) handleSessionModificationRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	var remoteSEID uint64

	sendError := func(err error) (message.Message, error) {
		log.Errorln(err)

		smres := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			remoteSEID,                           /* seid */
			smreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		)

		return smres, err
	}

	localSEID := smreq.SEID()

	session, ok := pConn.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("session not found: %v", localSEID))
	}

	var fseidIP uint32

	if smreq.CPFSEID != nil {
		fseid, err := smreq.CPFSEID.FSEID()
		if err == nil {
			session.remoteSEID = fseid.SEID
			fseidIP = ip2int(fseid.IPv4Address)

			log.Traceln("Updated FSEID from session modification request")
		}
	}

	remoteSEID = session.remoteSEID

	addPDRs := make([]pdr, 0, MaxItems)
	addFARs := make([]far, 0, MaxItems)
	addQERs := make([]qer, 0, MaxItems)
	endMarkerList := make([][]byte, 0, MaxItems)

	for _, cPDR := range smreq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, localSEID, pConn.appPFDs, upf.ippool); err != nil {
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
		var (
			p   pdr
			err error
		)

		if err = p.parsePDR(uPDR, localSEID, pConn.appPFDs, upf.ippool); err != nil {
			return sendError(err)
		}

		p.fseidIP = fseidIP

		err = session.UpdatePDR(p)
		if err != nil {
			log.Errorln("session PDR update failed ", err)
			continue
		}

		addPDRs = append(addPDRs, p)
	}

	for _, uFAR := range smreq.UpdateFAR {
		var (
			f   far
			err error
		)

		if err = f.parseFAR(uFAR, localSEID, upf, update); err != nil {
			return sendError(err)
		}

		f.fseidIP = fseidIP

		err = session.UpdateFAR(&f, &endMarkerList)
		if err != nil {
			log.Errorln("session PDR update failed ", err)
			continue
		}

		addFARs = append(addFARs, f)
	}

	for _, uQER := range smreq.UpdateQER {
		var (
			q   qer
			err error
		)

		if err = q.parseQER(uQER, localSEID, upf); err != nil {
			return sendError(err)
		}

		q.fseidIP = fseidIP

		err = session.UpdateQER(q)
		if err != nil {
			log.Errorln("session QER update failed ", err)
			continue
		}

		addQERs = append(addQERs, q)
	}

	session.MarkSessionQer()

	cause := upf.sendMsgToUPF(upfMsgTypeMod, addPDRs, addFARs, addQERs)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("write to FastPath failed"))
	}

	if session.getNotifyFlag() {
		session.updateNotifyFlag()
	}

	if upf.enableEndMarker {
		err := upf.sendEndMarkers(&endMarkerList)
		if err != nil {
			log.Errorln("Sending End Markers Failed : ", err)
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

	cause = upf.sendMsgToUPF(upfMsgTypeDel, delPDRs, delFARs, delQERs)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("write to FastPath failed"))
	}

	// Build response message
	smres := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		remoteSEID,                           /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	)

	return smres, nil
}

func (pConn *PFCPConn) handleSessionDeletionRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	sendError := func(err error) (message.Message, error) {
		smres := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			0,                                    /* seid */
			sdreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		)

		return smres, err
	}

	/* retrieve sessionRecord */
	localSEID := sdreq.SEID()

	session, ok := pConn.sessions[localSEID]
	if !ok {
		return sendError(fmt.Errorf("session not found: %v", localSEID))
	}

	cause := upf.sendMsgToUPF(upfMsgTypeDel, session.pdrs, session.fars, session.qers)
	if cause == ie.CauseRequestRejected {
		return sendError(errors.New("write to FastPath failed"))
	}

	if err := upf.ippool.DeallocIP(session.localSEID); err != nil {
		return sendError(errors.New("session IP dealloc failed"))
	}

	/* delete sessionRecord */
	pConn.RemoveSession(localSEID)

	// Build response message
	smres := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	)

	return smres, nil
}

func (pConn *PFCPConn) handleDigestReport(fseid uint64) {
	session, ok := pConn.sessions[fseid]
	if !ok {
		log.Warnln("No session found for fseid : ", fseid)
		return
	}

	/* Check if notify is already sent in current time interval */
	if session.getNotifyFlag() {
		return
	}

	seq := pConn.getSeqNum()
	srreq := message.NewSessionReportRequest(0, /* MO?? <-- what's this */
		0,                            /* FO <-- what's this? */
		0,                            /* seid */
		seq,                          /* seq # */
		0,                            /* priority */
		ie.NewReportType(0, 0, 0, 1), /*upir, erir, usar, dldr int*/
	)
	srreq.Header.SEID = session.remoteSEID

	var pdrID uint32

	var farID uint32

	for _, pdr := range session.pdrs {
		if pdr.srcIface == core {
			pdrID = pdr.pdrID

			farID = pdr.farID

			break
		}
	}

	for _, far := range session.fars {
		if far.farID == farID {
			if far.applyAction&ActionNotify == 0 {
				log.Errorln("packet recieved for forwarding far. discard")
				return
			}
		}
	}

	if pdrID == 0 {
		log.Errorln("No Pdr found for downlink")

		return
	}

	go session.runTimerForDDNNotify(DefaultDDNTimeout)

	session.setNotifyFlag(true)

	srreq.DownlinkDataReport = ie.NewDownlinkDataReport(
		ie.NewPDRID(uint16(pdrID)))

	pConn.SendPFCPMsg(srreq)
}

func (pConn *PFCPConn) handleSessionReportResponse(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	srres, ok := msg.(*message.SessionReportResponse)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	cause := srres.Cause.Payload[0]
	if cause == ie.CauseRequestAccepted {
		return nil, nil
	}

	log.Warnln("session req not accepted seq : ", srres.SequenceNumber)

	seid := srres.SEID()

	if cause == ie.CauseSessionContextNotFound {
		sessItem, ok := pConn.sessions[seid]
		if !ok {
			return nil, errProcess(
				fmt.Errorf("context not found locally or remote. SEID : %v", seid))
		}

		log.Warnln("context not found, deleting session locally")

		pConn.RemoveSession(seid)

		cause := upf.sendMsgToUPF(
			upfMsgTypeDel, sessItem.pdrs, sessItem.fars, sessItem.qers)
		if cause == ie.CauseRequestRejected {
			return nil, errProcess(
				fmt.Errorf("delete session from fastpath failed for: %v", seid))
		}

		return nil, nil
	}
	return nil, nil
}
