// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/message"
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

	msg, err := message.Parse(buf)
	if err != nil {
		log.Errorln("Ignoring undecodable message: ", buf, " error: ", err)
		return
	}

	addr := pConn.RemoteAddr().String()
	msgType := msg.MessageTypeName()

	log.Traceln("Received", msgType, "from", addr)

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

	nodeID := pConn.mgr.nodeID
	//nodeID := addr

	// Check for errors in handling the message
	if err != nil {
		globalPfcpStats.messages.WithLabelValues(nodeID, msgType, "Incoming", "Failure").Inc()
		log.Errorln("Error handling PFCP message type", msgType, "from", addr, err)
	} else {
		globalPfcpStats.messages.WithLabelValues(nodeID, msgType, "Incoming", "Success").Inc()
		log.Traceln("Successfully processed", msgType, "from", addr)
	}

	if reply != nil {
		pConn.SendPFCPMsg(reply)
	}
}

func (pConn *PFCPConn) SendPFCPMsg(msg message.Message) {
	addr := pConn.RemoteAddr().String()
	nodeID := pConn.mgr.nodeID

	out := make([]byte, msg.MarshalLen())
	replyType := msg.MessageTypeName()

	if err := msg.MarshalTo(out); err != nil {
		globalPfcpStats.messages.WithLabelValues(nodeID, replyType, "Outgoing", "Failure").Inc()
		log.Errorln("Failed to marshal", replyType, "for", addr, err)
		return
	}

	if _, err := pConn.Write(out); err != nil {
		globalPfcpStats.messages.WithLabelValues(nodeID, replyType, "Outgoing", "Failure").Inc()
		log.Errorln("Failed to transmit", replyType, "to", addr, err)
		return
	}

	globalPfcpStats.messages.WithLabelValues(nodeID, replyType, "Outgoing", "Success").Inc()
	log.Traceln("Sent", replyType, "to", addr)
}
