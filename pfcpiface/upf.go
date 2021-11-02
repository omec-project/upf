// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

// QosConfigVal : Qos configured value.
type QosConfigVal struct {
	cbs              uint32
	pbs              uint32
	ebs              uint32
	burstDurationMs  uint32
	schedulePriority uint32
}

type SliceInfo struct {
	name         string
	uplinkMbr    uint64
	downlinkMbr  uint64
	ulBurstBytes uint64
	dlBurstBytes uint64
	ueResList    []UeResource
}

type UeResource struct {
	name string
	dnn  string
}

type upf struct {
	enableUeIPAlloc  bool
	enableEndMarker  bool
	accessIface      string
	coreIface        string
	ippoolCidr       string
	accessIP         net.IP
	coreIP           net.IP
	n4SrcIP          net.IP
	nodeIP           net.IP
	fqdnHost         string
	ippool           *IPPool
	recoveryTime     time.Time
	dnn              string
	reportNotifyChan chan uint64
	sliceInfo        *SliceInfo

	fastPath
}

// to be replaced with go-pfcp structs

// Don't change these values.
const (
	tunnelGTPUPort = 2152
	invalidQerID   = 0xFFFFFFFF

	// src-iface consts.
	core   = 0x2
	access = 0x1

	// far-id specific directions.
	n3 = 0x0
	n6 = 0x1
	n9 = 0x2
)

func (u *upf) isConnected() bool {
	return u.fastPath.isConnected(&u.accessIP)
}

func (u *upf) addSliceInfo(sliceInfo *SliceInfo) error {
	if sliceInfo == nil {
		return errors.New("invalid slice")
	}

	u.sliceInfo = sliceInfo

	return u.fastPath.addSliceInfo(sliceInfo)
}

func (u *upf) setUpfInfo(conf *Conf) {
	var err error

	u.reportNotifyChan = make(chan uint64, 1024)
	u.n4SrcIP = net.ParseIP(net.IPv4zero.String())
	u.nodeIP = net.ParseIP(net.IPv4zero.String())

	if conf.CPIface.SrcIP == "" {
		if conf.CPIface.DestIP != "" {
			log.Println("Dest address ", conf.CPIface.DestIP)
			u.n4SrcIP = getLocalIP(conf.CPIface.DestIP)
			log.Println("SPGWU/UPF address IP: ", u.n4SrcIP.String())
		}
	} else {
		addrs, err := net.LookupHost(conf.CPIface.SrcIP)
		if err == nil {
			u.n4SrcIP = net.ParseIP(addrs[0])
		}
	}

	if conf.CPIface.FQDNHost != "" {
		ips, err := net.LookupHost(conf.CPIface.FQDNHost)
		if err == nil {
			u.nodeIP = net.ParseIP(ips[0])
		}
	}

	log.Println("UPF Node IP : ", u.nodeIP.String())
	log.Println("UPF Local IP : ", u.n4SrcIP.String())

	u.ippoolCidr = conf.CPIface.UeIPPool

	log.Println("IP pool : ", u.ippoolCidr)

	u.ippool, err = NewIPPool(u.ippoolCidr)
	if err != nil {
		log.Println("ip pool init failed")
	}

	u.accessIP = ParseIP(conf.AccessIface.IfName, "Access")
	u.coreIP = ParseIP(conf.CoreIface.IfName, "Core")

	u.fastPath.setUpfInfo(u, conf)
}

func (u *upf) sim(method string, s *SimModeInfo) {
	log.Println(*simulate, "sessions:", s.MaxSessions)
	start := time.Now()
	// const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	ueip := s.StartUEIP
	enbip := s.StartENBIP
	aupfip := s.StartAUPFIP
	n9appip := s.N9AppIP
	n3TEID := hex2int(s.StartN3TEID)
	n9TEID := hex2int(s.StartN9TEID)

	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80

	for i := uint32(0); i < s.MaxSessions; i++ {
		// NG4T-based formula to calculate enodeB IP address against a given UE IP address
		// il_trafficgen also uses the same scheme
		// See SimuCPEnbv4Teid(...) in ngic code for more details
		ueOfRan := i % ng4tMaxUeRan
		ran := i / ng4tMaxUeRan
		enbOfRan := ueOfRan % ng4tMaxEnbRan
		enbIdx := ran*ng4tMaxEnbRan + enbOfRan

		// create/delete downlink pdr
		pdrN6Down := pdr{
			srcIface: core,
			dstIP:    ip2int(ueip) + i,

			srcIfaceMask: 0xFF,
			dstIPMask:    0xFFFFFFFF,

			precedence: 255,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n3,
			qerIDList: []uint32{n6, 1},
			needDecap: 0,
		}

		pdrN9Down := pdr{
			srcIface:     core,
			tunnelTEID:   n9TEID + i,
			tunnelIP4Dst: ip2int(u.coreIP),

			srcIfaceMask:     0xFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			tunnelIP4DstMask: 0xFFFFFFFF,

			precedence: 1,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n3,
			qerIDList: []uint32{n9, 1},
			needDecap: 1,
		}

		// create/delete uplink pdr
		pdrN6Up := pdr{
			srcIface:     access,
			tunnelIP4Dst: ip2int(u.accessIP),
			tunnelTEID:   n3TEID + i,
			srcIP:        ip2int(ueip) + i,

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			srcIPMask:        0xFFFFFFFF,

			precedence: 255,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n6,
			qerIDList: []uint32{n6, 1},
			needDecap: 1,
		}

		pdrN9Up := pdr{
			srcIface:     access,
			tunnelIP4Dst: ip2int(u.accessIP),
			tunnelTEID:   n3TEID + i,
			dstIP:        ip2int(n9appip),

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			dstIPMask:        0xFFFFFFFF,

			precedence: 1,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n9,
			qerIDList: []uint32{n9, 1},
			needDecap: 1,
		}

		pdrs := []pdr{pdrN6Down, pdrN9Down, pdrN6Up, pdrN9Up}

		// create/delete downlink far
		farDown := far{
			farID: n3,
			fseID: uint64(n3TEID + i),

			applyAction:  ActionForward,
			dstIntf:      ie.DstInterfaceAccess,
			tunnelType:   0x1,
			tunnelIP4Src: ip2int(u.accessIP),
			tunnelIP4Dst: ip2int(enbip) + enbIdx,
			tunnelTEID:   n3TEID + i,
			tunnelPort:   tunnelGTPUPort,
		}

		// create/delete uplink far
		farN6Up := far{
			farID: n6,
			fseID: uint64(n3TEID + i),

			applyAction: ActionForward,
			dstIntf:     ie.DstInterfaceCore,
		}

		farN9Up := far{
			farID: n9,
			fseID: uint64(n3TEID + i),

			applyAction:  ActionForward,
			dstIntf:      ie.DstInterfaceCore,
			tunnelType:   0x1,
			tunnelIP4Src: ip2int(u.coreIP),
			tunnelIP4Dst: ip2int(aupfip),
			tunnelTEID:   n9TEID + i,
			tunnelPort:   tunnelGTPUPort,
		}

		fars := []far{farDown, farN6Up, farN9Up}

		// create/delete uplink qer
		qerN6 := qer{
			qerID: n6,
			fseID: uint64(n3TEID + i),
			qfi:   9,
			ulGbr: 50000,
			ulMbr: 90000,
			dlGbr: 60000,
			dlMbr: 80000,
		}

		qerN9 := qer{
			qerID: n9,
			fseID: uint64(n3TEID + i),
			qfi:   8,
			ulGbr: 50000,
			ulMbr: 60000,
			dlGbr: 70000,
			dlMbr: 90000,
		}

		qers := []qer{qerN6, qerN9}

		// create/delete session qers
		sessionQer := qer{
			qerID:    1,
			fseID:    uint64(n3TEID + i),
			qosLevel: SessionQos,
			qfi:      0,
			ulGbr:    0,
			ulMbr:    100000,
			dlGbr:    0,
			dlMbr:    500000,
		}

		qers = append(qers, sessionQer)

		switch method {
		case "create":
			u.sendMsgToUPF(upfMsgTypeAdd, pdrs, fars, qers)

		case "delete":
			u.sendMsgToUPF(upfMsgTypeDel, pdrs, fars, qers)

		default:
			log.Fatalln("Unsupported method", method)
		}
	}
	log.Println("Sessions/s:", float64(s.MaxSessions)/time.Since(start).Seconds())
}
