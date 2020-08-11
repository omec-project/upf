// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"
	"strings"
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
	readTimeout = 2500 * time.Second
)

type sessRecord struct {
	pdrs []pdr
	fars []far
}

var sessions map[uint64]sessRecord

func pfcpifaceMainLoop(intf common, upf *upf, accessIP string, coreIP string, sourceIP string) {
	log.Println("pfcpifaceMainLoop@" + upf.fqdnHost + " says hello!!!")
	log.Println("n4 ip ", sourceIP)
	log.Println("access ip ", accessIP)

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
	cpConnected := false

	// cleanup the pipeline
	cleanupSessions := func() {
		if cpConnected {
			intf.sendDeleteAllSessionsMsgtoUPF(upf)
			cpConnected = false
		}
	}

	// Initialize pkt buf
	buf := make([]byte, PktBufSz)
	// Initialize pkt header

	// Initialize sessions map
	sessions = make(map[uint64]sessRecord)

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
			sourceIP = getOutboundIP(addrString[0]).String()
			log.Println("Source IP address is now: ", sourceIP)
		}

		//log.Println("Message: ", msg)

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			outgoingMessage = handleAssociationSetupRequest(msg, addr, sourceIP, accessIP, coreIP)
			cpConnected = true
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = handleSessionEstablishmentRequest(intf, upf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(intf, upf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(intf, upf, msg, addr, sourceIP)
		case message.MsgTypeAssociationReleaseRequest:
			outgoingMessage = handleAssociationReleaseRequest(msg, addr, sourceIP, accessIP)
			cleanupSessions()
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

func handleAssociationSetupRequest(msg message.Message, addr net.Addr, sourceIP string, accessIP string, coreIP string) []byte {
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
		//ie.NewUserPlaneIPResourceInformation(0x41, 0, coreIP, "", "", ie.SrcInterfaceCore),
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association setup response", err)
	}

	log.Println("Sent association setup response to: ", addr)

	return asres
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

func handleSessionEstablishmentRequest(intf common, upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)

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
		pdrs, fars, err := parsePDRsFARs(upf, sereq, fseid)
		if err != nil {
			cause = ie.CauseRequestRejected
		} else {
			cause = intf.sendMsgToUPF("add", pdrs, fars, upf)

			if cause == ie.CauseRequestAccepted {
				// Adding current session details to the hash map
				sessItem := sessRecord{
					pdrs: pdrs,
					fars: fars,
				}
				sessions[fseid.SEID] = sessItem
				cause = ie.CauseRequestAccepted
			}
		}
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

func handleSessionModificationRequest(intf common, upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
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
						upf.accessIP); f != nil {
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
			cause = intf.sendMsgToUPF("mod", nil, fars, upf)
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

func handleSessionDeletionRequest(intf common, upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)

	/* retrieve sessionRecord */
	sessItem := sessions[mySEID(sdreq.SEID())]

	var flag bool = intf.handleChannelStatus()

	var cause uint8 = ie.CauseRequestAccepted
	if !flag {
		cause = intf.sendMsgToUPF("del", sessItem.pdrs,
			sessItem.fars, upf)

		/* delete sessionRecord */
		delete(sessions, mySEID(sdreq.SEID()))
	}

	// Build response message
	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                    /* FO <-- what's this? */
		mySEID(sdreq.SEID()), /* seid */
		sdreq.SequenceNumber, /* seq # */
		0,                    /* priority */
		ie.NewCause(cause),   /* accept it blindly for the time being */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return smres
}
