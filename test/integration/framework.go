// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"

// this file should contain all the struct defs/constants used among different test cases.

const (
	defaultSliceID = 0

	defaultSDFFilter = "permit out udp from any to assigned 80-80"

	ueAddress    = "17.0.0.1"
	upfN3Address = "198.18.0.1"
	nodeBAddress = "198.18.0.10"

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8

	srcIfaceAccess = 0x1
	srcIfaceCore   = 0x2

	directionUplink   = 0x1
	directionDownlink = 0x2
)

type pfcpSessionData struct {
	nbAddress    string
	ueAddress    string
	upfN3Address string

	sdfFilter string

	precedence uint32

	ulTEID uint32
	dlTEID uint32

	sessQFI uint8
	appQFI  uint8
}

type p4RtEntries struct {
	sessionsUplink []*p4_v1.TableEntry
	sessionsDownlink []*p4_v1.TableEntry
	terminationsUplink []*p4_v1.TableEntry
	terminationsDownlink []*p4_v1.TableEntry

	tunnelPeers []*p4_v1.TableEntry

	applications []*p4_v1.TableEntry
}

func (e p4RtEntries) Len() int {
	return len(e.tunnelPeers) + len(e.applications) + len(e.sessionsUplink) +  len(e.sessionsDownlink) +
		len(e.terminationsUplink) + len(e.terminationsDownlink)
}