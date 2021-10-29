// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2021 Intel Corporation

package main

import (
	"errors"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

var errFlowDescAbsent = errors.New("flow description not present")
var errFastpathDown = errors.New("fastpath down")

func (pConn *PFCPConn) handleHeartbeatRequest(msg message.Message) (message.Message, error) {
	rTime := pConn.upf.recoveryTime

	hbreq, ok := msg.(*message.HeartbeatRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	// Build response message
	hbres := message.NewHeartbeatResponse(hbreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(rTime), /* ts */
	)

	return hbres, nil
}

func (pConn *PFCPConn) handleHeartbeatResponse(msg message.Message) (message.Message, error) {
	// TODO: Handle timers
	return nil, nil
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

	log.Infoln("Association Setup Request from", addr, "with nodeID", nodeID)

	ts, err := asreq.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		return nil, errUnmarshal(err)
	}

	log.Infoln("Association Setup Request from", addr, "with recovery timestamp:", ts)

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D

	networkInstance := string(ie.NewNetworkInstanceFQDN(upf.dnn).Payload)
	flags := uint8(0x41)

	if len(upf.dnn) != 0 {
		log.Infoln("Association Setup Response to", addr, "with DNN:", upf.dnn)
		// add ASSONI flag to set network instance.
		flags = uint8(0x61)
	}

	asres := message.NewAssociationSetupResponse(asreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(upf.recoveryTime),
		ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted),      /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(flags, 0, upf.accessIP.String(), "", networkInstance, ie.SrcInterfaceAccess),
		// ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
	) /* userplane ip resource info */

	if !upf.isConnected() {
		asres.Cause = ie.NewCause(ie.CauseRequestRejected)
		return asres, errProcess(errFastpathDown)
	}

	pConn.mgr.nodeID = nodeID

	features := make([]uint8, 4)

	if upf.enableUeIPAlloc {
		setUeipFeature(features...)
	}

	if upf.enableEndMarker {
		setEndMarkerFeature(features...)
	}

	asres.UPFunctionFeatures = ie.NewUPFunctionFeatures(features...)

	return asres, nil
}

func (pConn *PFCPConn) handleAssociationSetupResponse(msg message.Message) (message.Message, error) {
	addr := pConn.RemoteAddr().String()

	asres, ok := msg.(*message.AssociationSetupResponse)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	ts, err := asres.RecoveryTimeStamp.RecoveryTimeStamp()
	if err != nil {
		return nil, errUnmarshal(err)
	}

	log.Traceln("Received a PFCP association setup response with TS: ", ts, " from: ", addr)

	_, err = asres.Cause.Cause()
	if err != nil {
		return nil, errUnmarshal(err)
	}

	return nil, nil
}

func (pConn *PFCPConn) handleAssociationReleaseRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf
	rTime := upf.recoveryTime

	arreq, ok := msg.(*message.AssociationReleaseRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	arres := message.NewAssociationReleaseResponse(arreq.SequenceNumber,
		ie.NewRecoveryTimeStamp(rTime),
		ie.NewNodeID(upf.nodeIP.String(), "", ""), /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted),      /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, upf.accessIP.String(), "", "", ie.SrcInterfaceAccess),
	)

	return arres, nil
}

func (pConn *PFCPConn) handlePFDMgmtRequest(msg message.Message) (message.Message, error) {
	pfdmreq, ok := msg.(*message.PFDManagementRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	currentAppPFDs := pConn.mgr.appPFDs

	// On every PFD management request reset existing contents
	// TODO: Analyse impact on PDRs referencing these IDs
	pConn.mgr.ResetAppPFDs()

	errUnmarshalReply := func(err error, offendingIE *ie.IE) (message.Message, error) {
		// Revert the map to original contents
		pConn.mgr.appPFDs = currentAppPFDs
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

		pConn.mgr.NewAppPFD(id)
		appPFD := pConn.mgr.appPFDs[id]

		pfdCtx, err := appIDPFD.PFDContext()
		if err != nil {
			pConn.mgr.RemoveAppPFD(id)
			return errUnmarshalReply(err, appIDPFD)
		}

		for _, pfdContent := range pfdCtx {
			fields, err := pfdContent.PFDContents()
			if err != nil {
				pConn.mgr.RemoveAppPFD(id)
				return errUnmarshalReply(err, appIDPFD)
			}

			if fields.FlowDescription == "" {
				return errUnmarshalReply(errFlowDescAbsent, appIDPFD)
			}

			appPFD.flowDescs = append(appPFD.flowDescs, fields.FlowDescription)
		}

		pConn.mgr.appPFDs[id] = appPFD
		log.Traceln("Flow descriptions for AppID", id, ":", appPFD.flowDescs)
	}

	// Build response message
	pfdres := message.NewPFDManagementResponse(pfdmreq.SequenceNumber,
		ie.NewCause(ie.CauseRequestAccepted),
		nil,
	)

	return pfdres, nil
}

func (pConn *PFCPConn) manageSmfConnection(n4LocalIP string, n3ip string, n4Dst string, conn *net.UDPConn, cpConnectionStatus chan bool, rTime time.Time) {
	cpConnected := false

	initiatePfcpConnection := func() {
		log.Traceln("SPGWC/SMF hostname ", n4Dst)
		n4DstIP := getRemoteIP(n4Dst)
		log.Traceln("SPGWC/SMF address IP inside manageSmfConnection ", n4DstIP.String())
		// initiate request if we have control plane address available
		if n4DstIP.String() != net.IPv4zero.String() {
			pConn.generateAssociationRequest(n4LocalIP, n3ip, n4DstIP.String(), conn, rTime)
		}
	}

	updateSmfStatus := func(msg bool) {
		log.Traceln("cpConnected : ", cpConnected, "msg ", msg)
		// events from main Loop
		if cpConnected && !msg {
			log.Warnln("CP disconnected ")

			cpConnected = false
		} else if !cpConnected && msg {
			log.Infoln("CP Connected ")

			cpConnected = true
		} else {
			log.Infoln("cpConnected ", cpConnected, "msg - ", msg)
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
				log.Infoln("Retry pfcp connection setup ", n4Dst)
				initiatePfcpConnection()
			}
		case <-pfcpResponseTicker.C:
			// we will attempt new connection after next recheck
			log.Warnln("PFCP session setup timeout ")
			pfcpResponseTicker.Stop()
		}
	}
}

func (pConn *PFCPConn) generateAssociationRequest(n4LocalIP string, n3ip string, n4DstIP string, conn *net.UDPConn, rTime time.Time) {
	log.Infoln("n4DstIp ", n4DstIP)

	seq := pConn.getSeqNum()
	// Build request message
	asreq, err := message.NewAssociationSetupRequest(seq, ie.NewRecoveryTimeStamp(rTime),
		ie.NewNodeID(n4LocalIP, "", ""), /* node id (IPv4) */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, n3ip, "", "", ie.SrcInterfaceAccess),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Errorln("Unable to create association setup response", err)
	}

	smfAddr, err := net.ResolveUDPAddr("udp", n4DstIP+":"+PFCPPort)
	if err != nil {
		log.Errorln("Unable to resolve udp addr!", err)
		return
	}

	log.Infoln("SMF address ", smfAddr)

	if _, err := conn.WriteTo(asreq, smfAddr); err != nil {
		log.Errorln("Unable to transmit association setup request ", err)
	}
}
