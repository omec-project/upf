// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package main

import (
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func createPFCP(conn *net.UDPConn, raddr *net.UDPAddr) uint64 {
	{
		var seq uint32 = 0
		asreq, err := message.NewAssociationSetupRequest(
			seq,
			ie.NewRecoveryTimeStamp(time.Now()),
			ie.NewNodeID("127.0.0.1", "", ""),
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(asreq); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent association setup request to: %s", raddr)

		buf := make([]byte, 1500)
		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}

	{
		var seq uint32 = 1
		hbreq, err := message.NewHeartbeatRequest(
			seq,
			ie.NewRecoveryTimeStamp(time.Now()),
			nil,
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(hbreq); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent heartbeat request to: %s", raddr)

		buf := make([]byte, 1500)
		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}

	{
		var seq uint32 = 2
		asreq, err := message.NewPFDManagementRequest(
			seq,
			ie.NewApplicationIDsPFDs(
				ie.NewApplicationID("1000"),
				ie.NewPFDContext(
					ie.NewPFDContents("permit out ip from any to 6.6.6.6/32", "", "", "", "", nil, nil, nil),
					ie.NewPFDContents("permit in ip from 6.6.6.6/32 to any", "", "", "", "", nil, nil, nil),
				),
			),
			ie.NewApplicationIDsPFDs(
				ie.NewApplicationID("1001"),
				ie.NewPFDContext(
					ie.NewPFDContents("permit out 6 from 0.0.0.0 3000 to 192.168.96.0/24 2000", "", "", "", "", nil, nil, nil),
					ie.NewPFDContents("permit in 6 from 192.168.96.0/24 2000 to 0.0.0.0 3000", "", "", "", "", nil, nil, nil),
				),
			),
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(asreq); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent PFD management request to: %s", raddr)

		buf := make([]byte, 1500)
		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}

	{
		var seq uint32 = 3
		hbreq, err := message.NewSessionEstablishmentRequest(
			0,
			0,
			0,
			seq,
			0,
			ie.NewNodeID("127.0.0.1", "", ""),
			ie.NewFSEID(0x0000000000000001, net.ParseIP("127.0.0.1"), nil),
			ie.NewPDNType(ie.PDNTypeIPv4),
			// Uplink N9
			ie.NewCreatePDR(
				ie.NewPDRID(1),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x01, 0x30000000, net.ParseIP("198.18.0.1"), nil, 0),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(1),
				ie.NewQERID(1),
				ie.NewQERID(4),
			),
			// Uplink N6
			ie.NewCreatePDR(
				ie.NewPDRID(2),
				ie.NewPrecedence(50),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x01, 0x30000000, net.ParseIP("198.18.0.1"), nil, 0),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					// ie.NewSDFFilter("permit out ip from 6.6.6.6/32 to assigned", "", "", "", 2),
					ie.NewApplicationID("1000"),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(2),
				ie.NewQERID(2),
				ie.NewQERID(4),
			),
			// Downlink N9
			ie.NewCreatePDR(
				ie.NewPDRID(3),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewFTEID(0x01, 0x90000000, net.ParseIP("198.19.0.1"), nil, 0),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(3),
				ie.NewQERID(1),
				ie.NewQERID(4),
			),
			// Downlink N6
			ie.NewCreatePDR(
				ie.NewPDRID(4),
				ie.NewPrecedence(50),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
				),
				ie.NewFARID(3),
				ie.NewQERID(2),
				ie.NewQERID(4),
			),
			// Uplink N9
			ie.NewCreateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(0x02),
				ie.NewForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceCore),
					ie.NewOuterHeaderCreation(0x100, 0x00000001, "198.20.0.1", "", 0, 0, 0),
				),
			),
			// Uplink N6
			ie.NewCreateFAR(
				ie.NewFARID(2),
				ie.NewApplyAction(0x02),
				ie.NewForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceCore),
				),
			),
			// Downlink
			ie.NewCreateFAR(
				ie.NewFARID(3),
				ie.NewApplyAction(0x0c),
				// ie.NewApplyAction(0x02),
				// ie.NewForwardingParameters(
				// 	 ie.NewDestinationInterface(ie.DstInterfaceAccess),
				// 	 ie.NewOuterHeaderCreation(0x100, 0x00000001, "11.1.1.129", "", 0, 0, 0),
				// ),
			),
			// Session AMBR
			ie.NewCreateQER(
				ie.NewQERID(4),
				ie.NewQFI(0x09),
				ie.NewGateStatus(0, 0),
				ie.NewMBR(500000, 500000),
				ie.NewGBR(0, 0),
			),
			// Uplink N9
			ie.NewCreateQER(
				ie.NewQERID(1),
				ie.NewQFI(0x09),
				ie.NewGateStatus(0, 0),
				ie.NewMBR(50000, 50000),
				ie.NewGBR(30000, 30000),
			),
			// Uplink N6
			ie.NewCreateQER(
				ie.NewQERID(2),
				ie.NewQFI(0x08),
				ie.NewGateStatus(0, 0),
				ie.NewMBR(50000, 50000),
				ie.NewGBR(30000, 30000),
			),
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(hbreq); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent session establishment request to: %s", raddr)

		buf := make([]byte, 1500)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
		msg, err := message.Parse(buf[:n])
		if err != nil {
			log.Fatalln("Unable to parse session establishment response", err)
		}
		seres, ok := msg.(*message.SessionEstablishmentResponse)
		if !ok {
			log.Fatalln("Got an unexpected message: ", msg.MessageTypeName(), " from: ", addr)
		}
		fseid, err := seres.UPFSEID.FSEID()
		if err != nil {
			log.Fatalln("Failed to parse FSEID from session establishment response")
		}
		return fseid.SEID
	}
}

func modifyPFCP(conn *net.UDPConn, raddr *net.UDPAddr, seid uint64) {
	{
		var seq uint32 = 4
		hbreq, err := message.NewSessionModificationRequest(
			0,
			0,
			seid,
			seq,
			0,
			ie.NewPDNType(ie.PDNTypeIPv4),
			// Downlink N9
			ie.NewUpdatePDR(
				ie.NewPDRID(3),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewFTEID(0x01, 0x90000000, net.ParseIP("198.19.0.1"), nil, 0),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(3),
				ie.NewQERID(1),
				ie.NewQERID(4),
			),
			// Downlink N6
			ie.NewUpdatePDR(
				ie.NewPDRID(4),
				ie.NewPrecedence(50),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
				),
				ie.NewFARID(3),
				ie.NewQERID(2),
				ie.NewQERID(4),
			),
			// Downlink
			ie.NewUpdateFAR(
				ie.NewFARID(3),
				ie.NewApplyAction(0x02),
				ie.NewUpdateForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceAccess),
					ie.NewOuterHeaderCreation(0x100, 0x00000001, "11.1.1.129", "", 0, 0, 0),
				),
			),
			/*ie.NewUpdateQER(
				ie.NewQERID(1),
				ie.NewQFI(0x06),
				ie.NewGateStatus(0, 0),
				ie.NewMBR(50000, 50000),
				ie.NewGBR(30000, 30000),
			),*/
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(hbreq); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent session modification request to: %s", raddr)

		buf := make([]byte, 1500)
		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func deletePFCP(conn *net.UDPConn, raddr *net.UDPAddr, seid uint64) {
	{
		var seq uint32 = 5

		sdreq, err := message.NewSessionDeletionRequest(
			0,
			0,
			seid,
			seq,
			0,
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(sdreq); err != nil {
			log.Fatal(err)
		}

		log.Printf("sent session deletion request to: %s", raddr)

		buf := make([]byte, 1500)

		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}

	{
		var seq uint32 = 6
		arreq, err := message.NewAssociationReleaseRequest(
			seq,
			ie.NewNodeID("127.0.0.1", "", ""),
		).Marshal()
		if err != nil {
			log.Fatal(err)
		}

		if _, err := conn.Write(arreq); err != nil {
			log.Fatal(err)
		}

		log.Printf("sent association release request to: %s", raddr)

		buf := make([]byte, 1500)
		_, _, err = conn.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func pfcpSim() {
	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:"+PFCPPort)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Fatal(err)
	}

	seid := createPFCP(conn, raddr)

	time.Sleep(10 * time.Second)
	modifyPFCP(conn, raddr, seid)

	time.Sleep(10 * time.Second)
	deletePFCP(conn, raddr, seid)
}
