// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
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
	MaxItems = 10
	Timeout  = 1000 * time.Millisecond
)

type sessRecord struct {
	pdrs []pdr
	fars []far
}

var sessions map[uint64]sessRecord

func parsePDRFromPFCPSessEstReqPayload(upf *upf, sereq *message.SessionEstablishmentRequest, fseid *ie.FSEIDFields) (pdrs []pdr, fars []far) {

	pdrList := make([]pdr, 0, MaxItems)
	farList := make([]far, 0, MaxItems)

	/* read PDR(s) */
	ies1, err := ie.ParseMultiIEs(sereq.Payload)
	if err != nil {
		log.Println("Failed to parse sereq for IEs!")
		return pdrList, farList
	}
	/*
	 * Iteratively go through all IEs. You can't use ie.CreatePDR or ie.CreateFAR since a single
	 * message can carry multiple CreatePDR & CreateFAR messages.
	 */
	for _, ie1 := range ies1 {
		switch ie1.Type {
		case ie.CreatePDR:
			var srcIface uint8
			var teid uint32
			var ueIP4 net.IP

			/* reset outerHeaderRemoval to begin with */
			outerHeaderRemoval := uint8(0)

			pdrID, err := ie1.PDRID()
			if err != nil {
				log.Println("Could not read PDR!")
				return pdrList, farList
			}

			pdi, err := ie1.PDI()
			if err != nil {
				log.Println("Could not read PDI!")
				return pdrList, farList
			}

			_, err = ie1.OuterHeaderRemoval()
			if err == nil {
				res, err := ie1.OuterHeaderRemovalDescription()
				if res == 0 && err == nil { // 0 == GTP-U/UDP/IPv4
					outerHeaderRemoval = 1
				}
			}

			for _, ie2 := range pdi {
				switch ie2.Type {
				case ie.SourceInterface:
					srcIface, err = ie2.SourceInterface()
					if err != nil {
						log.Println("Failed to parse Source Interface IE!")
					} else {
						if srcIface == ie.SrcInterfaceCPFunction {
							log.Println("Detected src interface cp function. Ignoring for the time being")
						}
					}
				case ie.FTEID:
					fteid, err := ie2.FTEID()
					if err != nil {
						log.Println("Failed to parse FTEID IE")
					} else {
						teid = fteid.TEID
					}
				case ie.UEIPAddress:
					ueip4, err := ie2.UEIPAddress()
					if err != nil {
						log.Println("Failed to parse UE IP address")
					} else {
						ueIP4 = ueip4.IPv4Address
					}
				case ie.SDFFilter:
					// Do nothing for the time being
				case ie.QFI:
					// Do nothing for the time being
				}
			}

			farID, err := ie1.FARID()
			if err != nil {
				log.Println("Could not read FAR ID!")
				return pdrList, farList
			}

			// uplink PDR may not have UE IP address IE
			// FIXIT/TODO/XXX Move this inside SrcInterfaceAccess IE check??
			var ueIP uint32
			var ueIPMask uint32
			if len(ueIP4) == 0 {
				ueIP = 0
				ueIPMask = 0
			} else {
				ueIP = ip2int(ueIP4)
				ueIPMask = 0xFFFFFFFF
			}

			// populated everything for PDR, and set the right pdr_
			if srcIface == ie.SrcInterfaceAccess {
				pdrU := pdr{
					srcIface:     access,
					eNBTeid:      teid,
					dstIP:        ueIP,
					srcIfaceMask: 0xFF,
					eNBTeidMask:  0xFFFFFFFF,
					dstIPMask:    ueIPMask,
					pdrID:        uint32(pdrID),
					fseID:        uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
					ctrID:        0,                  // ctrID currently not being set <--- FIXIT/TODO/XXX
					farID:        uint8(farID),
					needDecap:    outerHeaderRemoval,
				}
				pdrList = append(pdrList, pdrU)
			} else if srcIface == ie.SrcInterfaceCore {
				pdrD := pdr{
					srcIface:     core,
					srcIP:        ueIP,
					srcIfaceMask: 0xFF,
					srcIPMask:    ueIPMask,
					pdrID:        uint32(pdrID),
					fseID:        uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
					ctrID:        0,                  // ctrID currently not being set <--- FIXIT/TOOD/XXX
					farID:        uint8(farID),       // farID currently not being set <--- FIXIT/TODO/XXX
					needDecap:    outerHeaderRemoval,
				}
				pdrList = append(pdrList, pdrD)
			}

		case ie.CreateFAR:
			if err != nil {
				log.Println("Failed to parse FAR IE!")
			} else {
				farID, err := ie1.FARID()
				if err != nil {
					log.Println("Could not read FAR ID!")
					return pdrList, farList
				}
				/* Read outerheadercreation from payload (if it exists) */
				var eNBTeid uint32
				eNBIP := uint32(0)
				n6IP4 := uint32(0)
				tunnelType := uint8(0)
				ies2, err := ie1.ForwardingParameters()
				if err != nil {
					log.Println("Unable to find ForwardingParameters!")
				} else {
					for _, ie2 := range ies2 {
						switch ie2.Type {
						case ie.OuterHeaderCreation:
							outerheadercreationfields, err := ie2.OuterHeaderCreation()
							if err != nil {
								log.Println("Unable to parse OuterHeaderCreationFields!")
							} else {
								eNBTeid = outerheadercreationfields.TEID
								eNBIP = ip2int(outerheadercreationfields.IPv4Address)
								n6IP4 = ip2int(upf.n6IP)
								tunnelType = uint8(1)
							}
						}
					}
				}
				far := far{
					farID:       uint8(farID),       // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
					fseID:       uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
					action:      farForwardU,
					tunnelType:  tunnelType,
					s1uIP:       n6IP4,
					eNBIP:       eNBIP,
					eNBTeid:     eNBTeid,
					UDPGTPUPort: udpGTPUPort,
				}
				farList = append(farList, far)
			}

		}
	}

	return pdrList, farList
}

func pfcpifaceMainLoop(upf *upf, n3ip string, sourceIP string) {
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

	// Initialize pkt buf
	buf := make([]byte, PktBufSz)
	// Initialize sessions map
	sessions = make(map[uint64]sessRecord)

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

		//log.Println("Message: ", msg)

		// handle message
		var outgoingMessage []byte
		switch msg.MessageType() {
		case message.MsgTypeAssociationSetupRequest:
			outgoingMessage = handleAssociationSetupRequest(msg, addr, sourceIP, n3ip)
		case message.MsgTypeSessionEstablishmentRequest:
			outgoingMessage = handleSessionEstablishmentRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeSessionModificationRequest:
			outgoingMessage = handleSessionModificationRequest(upf, msg, addr, sourceIP)
		case message.MsgTypeHeartbeatRequest:
			outgoingMessage = handleHeartbeatRequest(msg, addr)
		case message.MsgTypeSessionDeletionRequest:
			outgoingMessage = handleSessionDeletionRequest(upf, msg, addr, sourceIP)
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

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	asres, err := message.NewAssociationSetupResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
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

	// Build response message
	// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
	arres, err := message.NewAssociationReleaseResponse(ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
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

func handleSessionEstablishmentRequest(upf *upf, msg message.Message, addr net.Addr, sourceIP string) []byte {
	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		log.Println("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		return nil
	}

	log.Println("Got a session establishment request from: ", addr)

	/* Read fseid from the IE */
	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		log.Println("Failed to parse FSEID from session establishment request")
		return nil
	}

	/* Read CreatePDRs and CreateFARs from payload */
	pdrs, fars := parsePDRFromPFCPSessEstReqPayload(upf, sereq, fseid)

	/* create context, pause daemon, insert PDR(s), and resume daemon */
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	upf.pauseAll()
	for _, pdr := range pdrs {
		upf.addPDR(ctx, done, pdr)
	}
	for _, far := range fars {
		upf.addFAR(ctx, done, far)
	}
	upf.resumeAll()

	// Adding current session details to the hash map
	sessItem := sessRecord{
		pdrs: pdrs,
		fars: fars,
	}
	sessions[fseid.SEID] = sessItem

	// Build response message
	seres, err := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		fseid.SEID,                           /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewNodeID(sourceIP, "", ""),       /* node id (IPv4) */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID((fseid.SEID<<2), net.ParseIP(sourceIP), nil, nil),
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
	fseid := (smreq.SEID() >> 2)

	/* read FAR(s). These can be multiple */
	ies1, err := ie.ParseMultiIEs(smreq.Payload)
	if err != nil {
		log.Println("Failed to parse smreq for IEs!")
	} else {
		/*
		 * Iteratively go through all IEs. You can't use ie.UpdateFAR since a single
		 * message can carry multiple UpdateFAR messages.
		 */
		for _, ie1 := range ies1 {
			switch ie1.Type {
			case ie.UpdateFAR:
				/* check for updatefar, and fetch FAR ID */
				farID, err := ie1.FARID()
				if err != nil {
					log.Println("Unable to find updateFAR's FAR ID!")
				}

				/* Read UpdateFAR from payload */
				ies2, err := ie1.UpdateForwardingParameters()
				if err != nil {
					log.Println("Unable to find UpdateForwardingParameters!")
				} else {
					for _, ie2 := range ies2 {
						switch ie2.Type {
						case ie.OuterHeaderCreation:
							outerheadercreationfields, err := ie2.OuterHeaderCreation()
							if err != nil {
								log.Println("Unable to parse OuterHeaderCreationFields!")
							} else {
								eNBTeid := outerheadercreationfields.TEID
								eNBIP := outerheadercreationfields.IPv4Address
								s1uIP4 := upf.n3IP
								far := far{
									farID:       uint8(farID),  // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
									fseID:       uint32(fseid), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
									action:      farForwardD,
									tunnelType:  0x1,
									s1uIP:       ip2int(s1uIP4),
									eNBIP:       ip2int(eNBIP),
									eNBTeid:     eNBTeid,
									UDPGTPUPort: udpGTPUPort,
								}
								/* create context */
								ctx, cancel := context.WithTimeout(context.Background(), Timeout)
								defer cancel()
								done := make(chan bool)
								// pause daemon, and then insert FAR, finally resume
								upf.pauseAll()
								upf.addFAR(ctx, done, far)
								upf.resumeAll()
							}
						case ie.DestinationInterface:
							// Do nothing for the time being
						}
					}
				}
			}
		}
	}

	// Build response message
	smres, err := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		(smreq.SEID() >> 2),                  /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		ie.NewFSEID((smreq.SEID()<<2), net.ParseIP(sourceIP), nil, nil),
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
	sessItem := sessions[(sdreq.SEID() >> 2)]

	/* create context */
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()
	done := make(chan bool)
	// pause daemon, and then delete FAR(s) and PDR(s), finally resume
	upf.pauseAll()
	for _, pdr := range sessItem.pdrs {
		upf.delPDR(ctx, done, pdr)
	}
	for _, far := range sessItem.fars {
		upf.delFAR(ctx, done, far)
	}
	upf.resumeAll()

	/* delete sessionRecord */
	delete(sessions, (sdreq.SEID() >> 2))

	// Build response message
	smres, err := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		(sdreq.SEID() >> 2),                  /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	).Marshal()

	if err != nil {
		log.Fatalln("Unable to create session deletion response", err)
	}

	log.Println("Sent session deletion response to: ", addr)

	return smres
}
