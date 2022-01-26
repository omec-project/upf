// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpsim

import (
	"github.com/wmnsk/go-pfcp/ie"
	"net"
)

type IEMethod uint8

const (
	Create IEMethod = iota
	Update
	Delete
)

const (
	dummyPrecedence = 100
)

var uplinkPDR = ie.NewCreatePDR(
	ie.NewPDRID(1),
	ie.NewPrecedence(dummyPrecedence),
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
)

// TODO: use builder pattern to create PDR IE
func NewUplinkPDR(method IEMethod, id uint16, teid uint32, n3address string,
	farID uint32, sessQerID uint32, appQerID uint32) *ie.IE {
	createFunc := ie.NewCreatePDR
	if method == Update {
		createFunc = ie.NewUpdatePDR
	}

	return createFunc(
		ie.NewPDRID(id),
		ie.NewPrecedence(dummyPrecedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(0x01, teid, net.ParseIP(n3address), nil, 0),
			ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
		),
		ie.NewOuterHeaderRemoval(0, 0),
		ie.NewFARID(farID),
		ie.NewQERID(appQerID),
		ie.NewQERID(sessQerID),
	)
}

func NewDownlinkPDR(method IEMethod, id uint16, ueAddress string,
	farID uint32, sessQerID uint32, appQerID uint32) *ie.IE {
	createFunc := ie.NewCreatePDR
	if method == Update {
		createFunc = ie.NewUpdatePDR
	}

	return createFunc(
		ie.NewPDRID(id),
		ie.NewPrecedence(dummyPrecedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, ueAddress, "", 0, 0),
			ie.NewSDFFilter("permit out ip from any to assigned", "", "", "", 1),
		),
		ie.NewFARID(farID),
		ie.NewQERID(appQerID),
		ie.NewQERID(sessQerID),
	)
}

func NewUplinkFAR(method IEMethod, id uint32, applyAction uint8) *ie.IE {
	createFunc := ie.NewCreateFAR
	if method == Update {
		createFunc = ie.NewUpdateFAR
	}

	return createFunc(
		ie.NewFARID(id),
		ie.NewApplyAction(applyAction),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceCore),
		),
	)
}

func NewDownlinkFAR(method IEMethod, id uint32, applyAction uint8, teid uint32, downlinkIP string) *ie.IE {
	createFunc := ie.NewCreateFAR
	if method == Update {
		createFunc = ie.NewUpdateFAR
	}

	return createFunc(
		ie.NewFARID(id),
		ie.NewApplyAction(applyAction),
		ie.NewUpdateForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewOuterHeaderCreation(0x100, teid, downlinkIP, "", 0, 0, 0),
		),
	)
}

func NewQER(method IEMethod, id uint32, qfi uint8, ulMbr uint64, dlMbr uint64, ulGbr uint64, dlGbr uint64) *ie.IE {
	createFunc := ie.NewCreateQER
	if method == Update {
		createFunc = ie.NewUpdateQER
	}

	return createFunc(
		ie.NewQERID(id),
		ie.NewQFI(qfi),
		// FIXME: we don't support gating, always OPEN
		ie.NewGateStatus(0, 0),
		ie.NewMBR(ulMbr, dlMbr),
		ie.NewGBR(ulGbr, dlGbr),
	)
}
