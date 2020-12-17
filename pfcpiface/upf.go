// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"net"
)

type upf struct {
	accessIface string
	coreIface   string
	accessIP    net.IP
	coreIP      net.IP
	n4SrcIP     net.IP
	fqdnHost    string
	maxSessions uint32
	simInfo     *SimModeInfo
	intf        fastPath
}

// to be replaced with go-pfcp structs

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

func (u *upf) sendMsgToUPF(method string, pdrs []pdr, fars []far) uint8 {
	return u.intf.sendMsgToUPF(method, pdrs, fars)
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

func (u *upf) sim(method string) {
	u.intf.sim(u, method)
}

func (u *upf) setUpfInfo(conf *Conf) {
	u.intf.setUpfInfo(u, conf)
}

func (u *upf) setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn) {
	u.intf.setInfo(udpConn, udpAddr, pconn)
}
