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
			SrcIface: core,
			AppFilter: applicationFilter{
				DstIP:     ip2int(ueip) + i,
				DstIPMask: 0xFFFFFFFF,
			},

			SrcIfaceMask: 0xFF,

			Precedence: 255,

			PdrID:     1,
			FseID:     uint64(n3TEID + i),
			CtrID:     i,
			FarID:     n3,
			QerIDList: []uint32{n6, 1},
			NeedDecap: 0,
		}

		pdrN9Down := pdr{
			SrcIface:     core,
			TunnelTEID:   n9TEID + i,
			TunnelIP4Dst: ip2int(u.coreIP),

			SrcIfaceMask:     0xFF,
			TunnelTEIDMask:   0xFFFFFFFF,
			TunnelIP4DstMask: 0xFFFFFFFF,

			Precedence: 1,

			PdrID:     2,
			FseID:     uint64(n3TEID + i),
			CtrID:     i,
			FarID:     n3,
			QerIDList: []uint32{n9, 1},
			NeedDecap: 1,
		}

		// create/delete uplink pdr
		pdrN6Up := pdr{
			SrcIface:     access,
			TunnelIP4Dst: ip2int(u.accessIP),
			TunnelTEID:   n3TEID + i,
			AppFilter: applicationFilter{
				SrcIP:     ip2int(ueip) + i,
				SrcIPMask: 0xFFFFFFFF,
			},

			SrcIfaceMask:     0xFF,
			TunnelIP4DstMask: 0xFFFFFFFF,
			TunnelTEIDMask:   0xFFFFFFFF,

			Precedence: 255,

			PdrID:     3,
			FseID:     uint64(n3TEID + i),
			CtrID:     i,
			FarID:     n6,
			QerIDList: []uint32{n6, 1},
			NeedDecap: 1,
		}

		pdrN9Up := pdr{
			SrcIface:     access,
			TunnelIP4Dst: ip2int(u.accessIP),
			TunnelTEID:   n3TEID + i,
			AppFilter: applicationFilter{
				DstIP:     ip2int(n9appip),
				DstIPMask: 0xFFFFFFFF,
			},

			SrcIfaceMask:     0xFF,
			TunnelIP4DstMask: 0xFFFFFFFF,
			TunnelTEIDMask:   0xFFFFFFFF,

			Precedence: 1,

			PdrID:     4,
			FseID:     uint64(n3TEID + i),
			CtrID:     i,
			FarID:     n9,
			QerIDList: []uint32{n9, 1},
			NeedDecap: 1,
		}

		pdrs := []pdr{pdrN6Down, pdrN9Down, pdrN6Up, pdrN9Up}

		// create/delete downlink far
		farDown := far{
			FarID: n3,
			FseID: uint64(n3TEID + i),

			ApplyAction:  ActionForward,
			DstIntf:      ie.DstInterfaceAccess,
			TunnelType:   0x1,
			TunnelIP4Src: ip2int(u.accessIP),
			TunnelIP4Dst: ip2int(enbip) + enbIdx,
			TunnelTEID:   n3TEID + i,
			TunnelPort:   tunnelGTPUPort,
		}

		// create/delete uplink far
		farN6Up := far{
			FarID: n6,
			FseID: uint64(n3TEID + i),

			ApplyAction: ActionForward,
			DstIntf:     ie.DstInterfaceCore,
		}

		farN9Up := far{
			FarID: n9,
			FseID: uint64(n3TEID + i),

			ApplyAction:  ActionForward,
			DstIntf:      ie.DstInterfaceCore,
			TunnelType:   0x1,
			TunnelIP4Src: ip2int(u.coreIP),
			TunnelIP4Dst: ip2int(aupfip),
			TunnelTEID:   n9TEID + i,
			TunnelPort:   tunnelGTPUPort,
		}

		fars := []far{farDown, farN6Up, farN9Up}

		// create/delete uplink qer
		qerN6 := qer{
			QerID: n6,
			FseID: uint64(n3TEID + i),
			Qfi:   9,
			UlGbr: 50000,
			UlMbr: 90000,
			DlGbr: 60000,
			DlMbr: 80000,
		}

		qerN9 := qer{
			QerID: n9,
			FseID: uint64(n3TEID + i),
			Qfi:   8,
			UlGbr: 50000,
			UlMbr: 60000,
			DlGbr: 70000,
			DlMbr: 90000,
		}

		qers := []qer{qerN6, qerN9}

		// create/delete session qers
		sessionQer := qer{
			QerID:    1,
			FseID:    uint64(n3TEID + i),
			QosLevel: SessionQos,
			Qfi:      0,
			UlGbr:    0,
			UlMbr:    100000,
			DlGbr:    0,
			DlMbr:    500000,
		}

		qers = append(qers, sessionQer)

		allRules := PacketForwardingRules{
			pdrs: pdrs,
			fars: fars,
			qers: qers,
		}

		if mode.create() {
			u.sendMsgToUPF(upfMsgTypeAdd, allRules, PacketForwardingRules{})
		} else if mode.delete() {
			u.sendMsgToUPF(upfMsgTypeDel, allRules, PacketForwardingRules{})
		} else {
			log.Fatalln("Unsupported method", mode)
		}
	}

	log.Infoln("Sessions/s:", float64(s.MaxSessions)/time.Since(start).Seconds())
}
