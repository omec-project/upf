// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// PktBufSz : buffer size for incoming pkt
const (
	PktBufSz = 1500
	PFCPPort = "8805"
)

func pfcpifaceMainLoop(n3ip string, sourceIP string) {
	log.Println("cpiface_daemon says hello!!!")

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

	buf := make([]byte, PktBufSz)
	for {
		// blocking read
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Fatalln("Unable to read packet buffer")
			return
		}
		// use wmnsk lib to parse the pfcp message
		msg, err := message.Parse(buf[:n])
		if err != nil {
			log.Println("Ignoring undecodable message: ", buf[:n], " error: ", err)
			continue
		}

		log.Println("Message: ", msg)

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			outgoingMessage = handleAssociationSetupRequest(msg, addr, sourceIP, n3ip)
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = handleSessionEstablishmentRequest(msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(msg, addr, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(msg, addr, sourceIP, n3ip)
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

func handleAssociationSetupRequest(msg message.Message, addr net.Addr, sourceIP string, n3ip string) []byte {
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

	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	asres, err := message.NewAssociationSetupResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, n3ip, "", "", ie.SrcInterfaceAccess),
		ie.NewSequenceNumber(asreq.SequenceNumber), /* seq # */
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association setup response", err)
	}

	log.Println("Sent association setup response to: ", addr)

	return asres
}

func handleAssociationReleaseRequest(msg message.Message, addr net.Addr, sourceIP string, n3ip string) []byte {
	arreq, ok := msg.(*message.AssociationReleaseRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got an association release request from: ", addr)

	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	arres, err := message.NewAssociationReleaseResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, n3ip, "", "", ie.SrcInterfaceAccess),
		ie.NewSequenceNumber(arreq.SequenceNumber), /* seq # */
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association release response", err)
	}

	log.Println("Sent association release response to: ", addr)

	return arres
}

func handleSessionEstablishmentRequest(msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)

	seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		sereq.SEID(),                         /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewNodeID(sourceIP, "", ""),       /* node id */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(sereq.SEID()<<2, net.ParseIP(sourceIP), nil, nil),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session establishment response", err)
	}

	log.Println("Sent session establishment response to: ", addr)

	return seres
}

func handleSessionModificationRequest(msg message.Message, addr net.Addr, sourceIP string) []byte {
	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session modification request from: ", addr)

	smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		smreq.SEID(),                         /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(smreq.SEID()<<2, net.ParseIP(sourceIP), nil, nil),
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

	hbres, err := message.NewHeartbeatResponse(ie.NewRecoveryTimeStamp(time.Now()), /* ts */
		ie.NewSequenceNumber(hbreq.SequenceNumber), /* seq # */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create heartbeat response", err)
	}

	log.Println("Sent heartbeat response to: ", addr)

	return hbres
}

func handleSessionDeletionRequest(msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)

	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		sdreq.SEID(),                         /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID(sdreq.SEID()<<2, net.ParseIP(sourceIP), nil, nil),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return smres
}
