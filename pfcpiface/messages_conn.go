// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package main

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

var errFlowDescAbsent = errors.New("flow description not present")
var errFastpathDown = errors.New("fastpath down")
var errReqRejected = errors.New("request rejected")

func (pConn *PFCPConn) sendAssociationRequest() {
	// Build request message
	asreq := message.NewAssociationSetupRequest(pConn.getSeqNum(),
		pConn.associationIEs()...,
	)

	r := newRequest(asreq)
	reply, timeout := pConn.sendPFCPRequestMessage(r)

	if reply != nil {
		err := pConn.handleAssociationSetupResponse(reply)
		if err != nil {
			log.Errorln("Handling of Assoc Setup Response Failed ", pConn.RemoteAddr())
			pConn.Shutdown()

			return
		}

		if pConn.upf.enableHBTimer {
			go pConn.startHeartBeatMonitor()
		}
	} else if timeout {
		pConn.Shutdown()
	}
}

func (pConn *PFCPConn) getHeartBeatRequest() *Request {
	seq := pConn.getSeqNum()

	hbreq := message.NewHeartbeatRequest(
		seq,
		ie.NewRecoveryTimeStamp(pConn.ts.local),
		nil,
	)

	return newRequest(hbreq)
}

func (pConn *PFCPConn) handleHeartbeatRequest(msg message.Message) (message.Message, error) {
	hbreq, ok := msg.(*message.HeartbeatRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	if pConn.upf.enableHBTimer {
		// reset heartbeat expiry timer
		// non-blocking write to channel
		select {
		case pConn.hbReset <- struct{}{}:
			// timer reset
		default:
			// channel full, log warning and ignore
			log.Warn("failed to reset heartbeat timer")
		}
	}

	// TODO: Check and update remote recovery timestamp

	// Build response message
	hbres := message.NewHeartbeatResponse(hbreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(pConn.ts.local), /* ts */
	)

	return hbres, nil
}

func (pConn *PFCPConn) handleIncomingResponse(msg message.Message) {
	req, ok := pConn.pendingReqs.Load(msg.Sequence())

	if ok {
		req.(*Request).reply <- msg
		pConn.pendingReqs.Delete(msg.Sequence())
	}
}

func (pConn *PFCPConn) associationIEs() []*ie.IE {
	upf := pConn.upf
	networkInstance := string(ie.NewNetworkInstanceFQDN(upf.dnn).Payload)
	flags := uint8(0x41)

	if len(upf.dnn) != 0 {
		log.Infoln("Association Setup with DNN:", upf.dnn)
		// add ASSONI flag to set network instance.
		flags = uint8(0x61)
	}

	features := make([]uint8, 4)

	if upf.enableUeIPAlloc {
		setUeipFeature(features...)
	}

	if upf.enableEndMarker {
		setEndMarkerFeature(features...)
	}

	ies := []*ie.IE{
		ie.NewRecoveryTimeStamp(pConn.ts.local),
		pConn.nodeID.localIE,
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(flags, 0, upf.accessIP.String(), "", networkInstance, ie.SrcInterfaceAccess),
		// ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
		ie.NewUPFunctionFeatures(features...),
	}

	return ies
}

func (pConn *PFCPConn) handleAssociationSetupRequest(msg message.Message) (message.Message, error) {
	addr := pConn.RemoteAddr().String()
	upf := pConn.upf

	asreq, ok := msg.(*message.AssociationSetupRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	nodeID, err := asreq.NodeID.NodeID()
	if err != nil {
		return nil, errUnmarshal(err)
	}

	ts, err := asreq.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		return nil, errUnmarshal(err)
	}

	// Build response message
	asres := message.NewAssociationSetupResponse(asreq.SequenceNumber,
		pConn.associationIEs()...)

	if !upf.isConnected() {
		asres.Cause = ie.NewCause(ie.CauseRequestRejected)
		return asres, errProcess(errFastpathDown)
	}

	if pConn.ts.remote.IsZero() {
		pConn.ts.remote = ts
		log.Infoln("Association Setup Request from", addr,
			"with recovery timestamp:", ts)
	} else if ts.After(pConn.ts.remote) {
		old := pConn.ts.remote
		pConn.ts.remote = ts
		log.Warnln("Association Setup Request from", addr,
			"with newer recovery timestamp:", ts, "older:", old)
	}

	pConn.nodeID.remote = nodeID
	asres.Cause = ie.NewCause(ie.CauseRequestAccepted)

	log.Infoln("Association setup done between nodes",
		"local:", pConn.nodeID.local, "remote:", pConn.nodeID.remote)

	return asres, nil
}

func (pConn *PFCPConn) handleAssociationSetupResponse(msg message.Message) error {
	addr := pConn.RemoteAddr().String()

	asres, ok := msg.(*message.AssociationSetupResponse)
	if !ok {
		return errUnmarshal(errMsgUnexpectedType)
	}

	cause, err := asres.Cause.Cause()
	if err != nil {
		return errUnmarshal(err)
	}

	if cause != ie.CauseRequestAccepted {
		log.Errorln("Association Setup Response from", addr,
			"with Cause:", cause)
		return errReqRejected
	}

	nodeID, err := asres.NodeID.NodeID()
	if err != nil {
		return errUnmarshal(err)
	}

	ts, err := asres.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		return errUnmarshal(err)
	}

	if pConn.ts.remote.IsZero() {
		pConn.ts.remote = ts
		log.Infoln("Association Setup Response from", addr,
			"with recovery timestamp:", ts)
	} else if ts.After(pConn.ts.remote) {
		old := pConn.ts.remote
		pConn.ts.remote = ts
		log.Warnln("Association Setup Response from", addr,
			"with newer recovery timestamp:", ts, "older:", old)
	}

	pConn.nodeID.remote = nodeID
	log.Infoln("Association setup done between nodes",
		"local:", pConn.nodeID.local, "remote:", pConn.nodeID.remote)

	return nil
}

func (pConn *PFCPConn) handleAssociationReleaseRequest(msg message.Message) (message.Message, error) {
	arreq, ok := msg.(*message.AssociationReleaseRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	// Build response message
	arres := message.NewAssociationReleaseResponse(arreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(pConn.ts.local),
		pConn.nodeID.localIE,
		ie.NewCause(ie.CauseRequestAccepted),
	)

	return arres, nil
}

func (pConn *PFCPConn) handlePFDMgmtRequest(msg message.Message) (message.Message, error) {
	pfdmreq, ok := msg.(*message.PFDManagementRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	currentAppPFDs := pConn.appPFDs

	// On every PFD management request reset existing contents
	// TODO: Analyse impact on PDRs referencing these IDs
	pConn.ResetAppPFDs()

	errUnmarshalReply := func(err error, offendingIE *ie.IE) (message.Message, error) {
		// Revert the map to original contents
		pConn.appPFDs = currentAppPFDs
		// Build response message
		pfdres := message.NewPFDManagementResponse(pfdmreq.SequenceNumber,
			ie.NewCause(ie.CauseRequestRejected),
			offendingIE,
		)

		return pfdres, errUnmarshal(err)
	}

	for _, appIDPFD := range pfdmreq.ApplicationIDsPFDs {
		id, err := appIDPFD.ApplicationID()
		if err != nil {
			return errUnmarshalReply(err, appIDPFD)
		}

		pConn.NewAppPFD(id)
		appPFD := pConn.appPFDs[id]

		pfdCtx, err := appIDPFD.PFDContext()
		if err != nil {
			pConn.RemoveAppPFD(id)
			return errUnmarshalReply(err, appIDPFD)
		}

		for _, pfdContent := range pfdCtx {
			fields, err := pfdContent.PFDContents()
			if err != nil {
				pConn.RemoveAppPFD(id)
				return errUnmarshalReply(err, appIDPFD)
			}

			if fields.FlowDescription == "" {
				return errUnmarshalReply(errFlowDescAbsent, appIDPFD)
			}

			appPFD.flowDescs = append(appPFD.flowDescs, fields.FlowDescription)
		}

		pConn.appPFDs[id] = appPFD
		log.Traceln("Flow descriptions for AppID", id, ":", appPFD.flowDescs)
	}

	// Build response message
	pfdres := message.NewPFDManagementResponse(pfdmreq.SequenceNumber,
		ie.NewCause(ie.CauseRequestAccepted),
		nil,
	)

	return pfdres, nil
}
