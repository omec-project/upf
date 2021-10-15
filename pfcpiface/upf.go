// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
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
	maxSessions      uint32
	connTimeout      time.Duration
	readTimeout      time.Duration
	simInfo          *SimModeInfo
	intf             fastPath
	ippool           ipPool
	recoveryTime     time.Time
	dnn              string
	reportNotifyChan chan uint64
	sliceInfo        *SliceInfo
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

func (u *upf) sendMsgToUPF(
	method upfMsgType, pdrs []pdr, fars []far, qers []qer) uint8 {
	return u.intf.sendMsgToUPF(method, pdrs, fars, qers)
}

func (u *upf) sendEndMarkers(endMarkerList *[][]byte) error {
	return u.intf.sendEndMarkers(endMarkerList)
}

func sendDeleteAllSessionsMsgtoUPF(u *upf) {
	u.intf.sendDeleteAllSessionsMsgtoUPF()
}

func (u *upf) isConnected() bool {
	return u.intf.isConnected(&u.accessIP)
}

func (u *upf) exit() {
	u.intf.exit()
}

func (u *upf) addSliceInfo(sliceInfo *SliceInfo) error {
	if sliceInfo == nil {
		return errors.New("invalid slice")
	}

	u.sliceInfo = sliceInfo

	return u.intf.addSliceInfo(sliceInfo)
}

func (u *upf) sim(method string) {
	u.intf.sim(u, method)
}

func (u *upf) setUpfInfo(conf *Conf) {
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
		addrs, errin := net.LookupHost(conf.CPIface.SrcIP)
		if errin == nil {
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

	u.intf.setUpfInfo(u, conf)
}

func (u *upf) setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn) {
	u.intf.setInfo(udpConn, udpAddr, pconn)
}
