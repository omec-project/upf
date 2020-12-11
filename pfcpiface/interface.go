// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"net"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
)

type dataPath interface {
	exit()
	parseFunc(conf *Conf)
	setSimInfo(conf *Conf)
	setInfo(udpConn *net.UDPConn, updAddr net.Addr, pconn *PFCPConn)
	getAccessIPStr(val *string)
	getFqdnHost() string
	getAccessIP() net.IP
	getCoreIP() net.IP
	getAccessIface() string
	getCoreIface() string
	getSimInfo() *SimModeInfo
	getCoreIPStr(val *string)
	getN4SrcIP(val *string)
	handleChannelStatus() bool
	portStats(ifname string) *pb.GetPortStatsResponse
	sendMsgToUPF(method string, pdrs []pdr, fars []far) uint8
	sendDeleteAllSessionsMsgtoUPF()
	measure(ifName string, f *pb.MeasureCommandGetSummaryArg) *pb.MeasureCommandGetSummaryResponse
}

// Don't change these values
const (
	tunnelGTPUPort = 2152

	// src-iface consts
	core   = 0x2
	access = 0x1

	// far-id specific directions
	n3 = 0x0
	n6 = 0x1
	n9 = 0x2

	// far-action specific values
	farForwardU = 0x0
	farForwardD = 0x1
	farDrop     = 0x2
)
