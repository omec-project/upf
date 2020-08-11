// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	//"context"
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
	readTimeout = 25 * time.Second
)

type sessRecord struct {
	pdrs []pdr
	fars []far
}

var sessions map[uint64]sessRecord

func pfcpifaceMainLoop(upf *upf, accessIP string, sourceIP string) {
	log.Println("pfcpifaceMainLoop@" + upf.fqdnHost + " says hello!!!")

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
			sendDeleteAllSessionsMsgtoUPF(upf)
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
			outgoingMessage = handleAssociationSetupRequest(msg, addr, sourceIP, accessIP)
			cpConnected = true
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = handleSessionEstablishmentRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(upf, msg, addr, sourceIP)
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

func channel_setup() (*P4rtClient, error) {
    log.Println("Channel Setup.")
    localclient, err := CreateChannel(host, deviceId, timeout)
    if err != nil {
        log.Println("create channel failed : %v\n", err)
        return nil, err
    }
    if localclient != nil {
        log.Println("device id %d\n", (*localclient).DeviceID)
        err = SetSwitchInfo(conf)
        if err != nil {
            log.Println("Switch set info failed. %v\n", err)
            return nil, err
        }
    } else {
        log.Println("p4runtime client is null.\n")
        return nil, err
    }

    return localclient, nil
}

func handleAssociationSetupRequest(msg message.Message, addr net.Addr, sourceIP string, accessIP string) []byte {
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
	asres, err := message.NewAssociationSetupResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, accessIP, "", "", ie.SrcInterfaceAccess),
		ie.NewSequenceNumber(asreq.SequenceNumber), /* seq # */
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
	arres, err := message.NewAssociationReleaseResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		// 0x41 = Spare (0) | Assoc Src Inst (1) | Assoc Net Inst (0) | Tied Range (000) | IPV6 (0) | IPV4 (1)
		//      = 01000001
		ie.NewUserPlaneIPResourceInformation(0x41, 0, accessIP, "", "", ie.SrcInterfaceAccess),
		ie.NewSequenceNumber(arreq.SequenceNumber), /* seq # */
	).Marshal() /* userplane ip resource info */
	if err != nil {
		log.Fatalln("Unable to create association release response", err)
	}

	log.Println("Sent association release response to: ", addr)

	return arres
}

func handleSessionEstablishmentRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)

	var fail bool = false
	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		log.Println("Failed to parse FSEID from session establishment request")
		return nil
	}

    if enable_p4rt == true {
        if p4client == nil || p4client.CheckStatus() != Ready {
            p4client, err = channel_setup()
            if err != nil {
                log.Println("create channel failed : %v", err)
                fail = true
            }
        }
    }

	var cause uint8 = ie.CauseRequestAccepted
	if fail == true {
		cause = ie.CauseRequestRejected
	} else {
		/* Read CreatePDRs and CreateFARs from payload */
		pdrs, fars := parsePDRFromPFCPSessEstReqPayload(upf, sereq, fseid)

        if enable_p4rt == true {
           upf.insertEntryP4Upf(pdrs, fars)
        } else {
	        upf.sendMsgToUPF("add", pdrs, fars)
	    }

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

	// Build response message
	seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                              /* FO <-- what's this? */
		fseid.SEID,                     /* seid */
		sereq.SequenceNumber,           /* seq # */
		0,                              /* priority */
		ie.NewNodeID(sourceIP, "", ""), /* node id */
		ie.NewCause(cause),             /* accept it blindly for the time being */
		ie.NewFSEID(peerSEID(fseid.SEID), net.ParseIP(sourceIP), nil, nil),
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session establishment response", err)
	}

	log.Println("Sent session establishment response to: ", addr)

	return seres
}

func handleSessionModificationRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session modification request from: ", addr)

	/* fetch FSEID */

	fseid := (mySEID(smreq.SEID()))
	fars := make([]far, 0, MaxItems)

    var flag bool = false
    if enable_p4rt == true {
        var err error
        if p4client == nil || p4client.CheckStatus() != Ready {
            p4client, err = channel_setup()
            if err != nil {
                log.Println("create channel failed : %v", err)
                flag = true
            }
        }
    }

	var cause uint8 = ie.CauseRequestAccepted
    if flag == true {
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
                                upf.accessIP, farForwardD); f != nil {
					        fars = append(fars, *f)
				        }
			         default:
	                	/* more will be added later */
			    }
		    }
	    }

        if enable_p4rt == true {
            cause = upf.updateFarP4Upf(fars)
        } else {
            upf.sendMsgToUPF("add", nil, fars)
		}

    }
	// Build response message
	smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                    /* FO <-- what's this? */
		(mySEID(smreq.SEID())),  /* seid */
		smreq.SequenceNumber, /* seq # */
		0,                    /* priority */
		ie.NewCause(cause),   /* accept it blindly for the time being */
		ie.NewFSEID(peerSEID(smreq.SEID()), net.ParseIP(sourceIP), nil, nil),
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

	// Build response message
	hbres, err := message.NewHeartbeatResponse(ie.NewRecoveryTimeStamp(time.Now()), /* ts */
		ie.NewSequenceNumber(hbreq.SequenceNumber), /* seq # */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create heartbeat response", err)
	}

	log.Println("Sent heartbeat response to: ", addr)

	return hbres
}

func handleSessionDeletionRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session deletion request from: ", addr)

	/* retrieve sessionRecord */
	sessItem := sessions[mySEID(sdreq.SEID())]

    var flag bool = false
    if enable_p4rt == true {
        var err error
        if p4client == nil || p4client.CheckStatus() != Ready {
            p4client, err = channel_setup()
            if err != nil {
                log.Println("create channel failed : %v", err)
                flag = true
            }
        }
    }

	var cause uint8 = ie.CauseRequestAccepted
    if flag == false {
        if enable_p4rt == true {
            upf.deleteEntryP4Upf(sessItem.pdrs, sessItem.fars)
        } else {
	        upf.sendMsgToUPF("del", sessItem.pdrs, sessItem.fars)
	    }

        /* delete sessionRecord */
        delete(sessions, mySEID(sdreq.SEID()))
    }

	// Build response message
	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		mySEID(sdreq.SEID()),                 /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(cause), /* accept it blindly for the time being */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return smres
}
