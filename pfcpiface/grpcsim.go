// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package pfcpiface

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

// simMode : Type indicating the desired simulation mode.
type simMode int

const (
	simModeDisable simMode = iota
	simModeCreate
	simModeDelete
	simModeCreateAndContinue
)

func (s *simMode) String() string {
	switch *s {
	case simModeDisable:
		return "disable"
	case simModeCreate:
		return "create"
	case simModeDelete:
		return "delete"
	case simModeCreateAndContinue:
		return "create_continue"
	default:
		return "unknown sim mode"
	}
}

func (s *simMode) Set(value string) error {
	switch value {
	case "disable":
		*s = simModeDisable
	case "create":
		*s = simModeCreate
	case "delete":
		*s = simModeDelete
	case "create_continue":
		*s = simModeCreateAndContinue
	default:
		return ErrInvalidArgument("sim mode", value)
	}

	return nil
}

func (s simMode) create() bool {
	return s == simModeCreate || s == simModeCreateAndContinue
}

func (s simMode) delete() bool {
	return s == simModeDelete
}

func (s simMode) keepGoing() bool {
	return s == simModeCreateAndContinue
}

func (s simMode) enable() bool {
	return s != simModeDisable
}

func (u *upf) sim(mode simMode, s *SimModeInfo) {
	log.Infoln(simulate.String(), "sessions:", s.MaxSessions)

	start := time.Now()
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
			appFilter: applicationFilter{
				dstIP:     ip2int(ueip) + i,
				dstIPMask: 0xFFFFFFFF,
			},

			srcIfaceMask: 0xFF,

			precedence: 255,

			pdrID:     1,
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

			pdrID:     2,
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
			appFilter: applicationFilter{
				srcIP:     ip2int(ueip) + i,
				srcIPMask: 0xFFFFFFFF,
			},

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,

			precedence: 255,

			pdrID:     3,
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
			appFilter: applicationFilter{
				dstIP:     ip2int(n9appip),
				dstIPMask: 0xFFFFFFFF,
			},

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,

			precedence: 1,

			pdrID:     4,
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

		allRules := PacketForwardingRules{
			pdrs: pdrs,
			fars: fars,
			qers: qers,
		}

		if mode.create() {
			u.SendMsgToUPF(upfMsgTypeAdd, allRules, PacketForwardingRules{})
		} else if mode.delete() {
			u.SendMsgToUPF(upfMsgTypeDel, allRules, PacketForwardingRules{})
		} else {
			log.Fatalln("Unsupported method", mode)
		}
	}

	log.Infoln("Sessions/s:", float64(s.MaxSessions)/time.Since(start).Seconds())
}
