// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"flag"
	"log"
	"net"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func createPFCP(conn *net.UDPConn, raddr *net.UDPAddr) uint64 {
	{
		var seq uint32 = 1
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
		var seq uint32 = 2
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
		var seq uint32 = 3
		hbreq, err := message.NewSessionEstablishmentRequest(
			0,
			0,
			0,
			seq,
			0,
			ie.NewNodeID("127.0.0.1", "", ""),
			ie.NewFSEID(0x0000000000000001, net.ParseIP("127.0.0.1"), nil, nil),
			ie.NewPDNType(ie.PDNTypeIPv4),
			// Uplink N9
			ie.NewCreatePDR(
				ie.NewPDRID(1),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x30000000, net.ParseIP("198.18.0.1"), nil, nil),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(1),
			),
			// Uplink N6
			ie.NewCreatePDR(
				ie.NewPDRID(2),
				ie.NewPrecedence(50),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x30000000, net.ParseIP("198.18.0.1"), nil, nil),
					ie.NewUEIPAddress(0x2, "16.0.0.1", "", 0, 0),
					ie.NewSDFFilter("permit out ip from 6.6.6.6/32 to assigned", "", "", "", 2),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(2),
			),
			// Downlink N9
			ie.NewCreatePDR(
				ie.NewPDRID(3),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewFTEID(0x90000000, net.ParseIP("198.19.0.1"), nil, nil),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(3),
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
				//ie.NewApplyAction(0x02),
				//ie.NewForwardingParameters(
				//	ie.NewDestinationInterface(ie.DstInterfaceAccess),
				//	ie.NewOuterHeaderCreation(0x100, 0x00000001, "11.1.1.129", "", 0, 0, 0),
				//),
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
			//ie.NewFSEID(0x0000000000000001, net.ParseIP("127.0.0.1"), nil, nil),
			ie.NewPDNType(ie.PDNTypeIPv4),
			// Downlink N9
			ie.NewUpdatePDR(
				ie.NewPDRID(3),
				ie.NewPrecedence(100),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewFTEID(0x90000000, net.ParseIP("198.19.0.1"), nil, nil),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(3),
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

func pfcpSim() {
	var (
		server = flag.String("-s", "127.0.0.1:8805", "server's addr/port")
	)
	flag.Parse()

	raddr, err := net.ResolveUDPAddr("udp", *server)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Fatal(err)
	}

	seid := createPFCP(conn, raddr)
	time.Sleep(20 * time.Second)

	modifyPFCP(conn, raddr, seid)
	time.Sleep(20 * time.Second)

	deletePFCP(conn, raddr, seid)

}
