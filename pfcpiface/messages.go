// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/message"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

var errMsgUnexpectedType = errors.New("unable to parse message as type specified")

type HandlePFCPMsgError struct {
	Op  string
	Err error
}

func (e *HandlePFCPMsgError) Error() string {
	return "Error during " + e.Op + e.Err.Error()
}

func errUnmarshal(err error) *HandlePFCPMsgError {
	return &HandlePFCPMsgError{Op: "Unmarshal", Err: err}
}

func errProcess(err error) *HandlePFCPMsgError {
	return &HandlePFCPMsgError{Op: "Process", Err: err}
}

// HandlePFCPMsg handles different types of PFCP messages.
func (pConn *PFCPConn) HandlePFCPMsg(buf []byte) {
	var reply message.Message
	var err error

	// Check that the PFCP message did not get truncated.
	header, err := message.ParseHeader(buf)
	if err != nil {
		log.Errorln("could not parse PFCP message header: ", err)
		return
	}
	if header.Length > uint16(len(buf)) {
		log.Errorln("receive buffer too small for PFCP message")
		return
	}

	msg, err := message.Parse(buf)
	if err != nil {
		log.Errorln("Ignoring undecodable message: ", buf, " error: ", err)
		return
	}

	addr := pConn.RemoteAddr().String()
	msgType := msg.MessageTypeName()
	m := metrics.NewMessage(msgType, "Incoming")

	switch msg.MessageType() {
	// Connection related messages
	case message.MsgTypeHeartbeatRequest:
		reply, err = pConn.handleHeartbeatRequest(msg)
	case message.MsgTypeHeartbeatResponse:
		reply, err = pConn.handleHeartbeatResponse(msg)
	case message.MsgTypePFDManagementRequest:
		reply, err = pConn.handlePFDMgmtRequest(msg)
	case message.MsgTypeAssociationSetupRequest:
		reply, err = pConn.handleAssociationSetupRequest(msg)
		// TODO: Cleanup sessions
		// TODO: start heartbeats
	case message.MsgTypeAssociationSetupResponse:
		reply, err = pConn.handleAssociationSetupResponse(msg)
		// TODO: Cleanup sessions
		// TODO: start heartbeats
	case message.MsgTypeAssociationReleaseRequest:
		reply, err = pConn.handleAssociationReleaseRequest(msg)
		defer pConn.Shutdown()

	// Session related messages
	case message.MsgTypeSessionEstablishmentRequest:
		reply, err = pConn.handleSessionEstablishmentRequest(msg)
	case message.MsgTypeSessionModificationRequest:
		reply, err = pConn.handleSessionModificationRequest(msg)
	case message.MsgTypeSessionDeletionRequest:
		reply, err = pConn.handleSessionDeletionRequest(msg)
	case message.MsgTypeSessionReportResponse:
		pConn.handleSessionReportResponse(msg)
	default:
		log.Errorln("Message type: ", msgType, " is currently not supported")
		return
	}

	nodeID := pConn.nodeID.remote
	// Check for errors in handling the message
	if err != nil {
		m.Finish(nodeID, "Failure")
		log.Errorln("Error handling PFCP message type", msgType, "from:", addr, "nodeID:", nodeID, err)
	} else {
		m.Finish(nodeID, "Success")
		log.Traceln("Successfully processed", msgType, "from", addr, "nodeID:", nodeID)
	}

	pConn.SaveMessages(m)

	if reply != nil {
		pConn.SendPFCPMsg(reply)
	}
}

func (pConn *PFCPConn) SendPFCPMsg(msg message.Message) {
	addr := pConn.RemoteAddr().String()
	nodeID := pConn.nodeID.remote
	msgType := msg.MessageTypeName()

	m := metrics.NewMessage(msgType, "Outgoing")
	defer pConn.SaveMessages(m)

	out := make([]byte, msg.MarshalLen())

	if err := msg.MarshalTo(out); err != nil {
		m.Finish(nodeID, "Failure")
		log.Errorln("Failed to marshal", msgType, "for", addr, err)
		return
	}

	if _, err := pConn.Write(out); err != nil {
		m.Finish(nodeID, "Failure")
		log.Errorln("Failed to transmit", msgType, "to", addr, err)
		return
	}

	m.Finish(nodeID, "Success")
	log.Traceln("Sent", msgType, "to", addr)
}
