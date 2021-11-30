// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"time"

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

type RequestTimeoutAction func(msg message.Message) bool

type Request struct {
	msg message.Message // Request message

	reply chan uint8 // Cause

	responseTimerDuration time.Duration

	shutdown <-chan struct{}
}

func newRequest(msg message.Message, done <-chan struct{}, respDur time.Duration) *Request {
	return &Request{msg: msg, reply: make(chan uint8, 1), shutdown: done, responseTimerDuration: respDur}
}

func (r *Request) GetResponse() (uint8, bool) {
	select {
	case <-r.shutdown:
		log.Traceln("Exiting as invoker routine aborted")
		return 0, false
	case c := <-r.reply:
		return c, false
	case <-time.After(r.responseTimerDuration):
		return 0, true
	}
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
		if reply != nil && err == nil {
			pConn.startHeartBeatMonitor()
		}
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

func (pConn *PFCPConn) WaitForResponse(r *Request, maxRetries uint8, timeoutHdlr RequestTimeoutAction) {
	pConn.pendingReqs.Store(r.msg.Sequence(), r)

	retriesLeft := maxRetries
	for {
		if _, rc := r.GetResponse(); rc {
			log.Traceln("Request Timeout, retriesLeft:", retriesLeft)
			if retriesLeft > 0 {
				pConn.SendPFCPMsg(r.msg)
				retriesLeft--
			} else {
				if timeoutHdlr != nil {
					timeoutHdlr(r.msg)
				}
				break
			}
		} else {
			log.Traceln("Exiting..")
			break
		}
	}
}

func GetCause(msg message.Message) uint8 {
	var cause uint8

	switch msg.MessageType() {
	case message.MsgTypeAssociationSetupResponse:
		cause, _ = msg.(*message.AssociationSetupResponse).Cause.Cause()
	case message.MsgTypeSessionReportResponse:
		cause, _ = msg.(*message.SessionReportResponse).Cause.Cause()
	}

	return cause

}

func (pConn *PFCPConn) RemovePendingRequest(msg message.Message) {
	req, ok := pConn.pendingReqs.Load(msg.Sequence())

	if ok {
		req.(*Request).reply <- GetCause(msg)
		pConn.pendingReqs.Delete(msg.Sequence())
	}

}
