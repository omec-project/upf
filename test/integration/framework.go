// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"errors"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"net"
	"time"
)

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

type portRange struct {
	low  uint16
	high uint16
}

type appFilter struct {
	proto        uint8
	appIP        net.IP
	appPrefixLen uint32
	appPort      portRange
}

type p4RtValues struct {
	ueAddress    string
	tunnelPeerID uint8
	appID        uint8
	appFilter    appFilter
}

func IsConnectionOpen(host string, port string) bool {
	ln, err := net.Listen("udp", host + ":" + port)
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

func waitForPFCPAgentToStart() error {
	timeout := time.After(5 * time.Second)
	ticker := time.Tick(500 * time.Millisecond)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker:
			ok := IsConnectionOpen("127.0.0.1", "8805")
			if ok {
				return nil
			}
		}
	}
}

// TimeBasedElectionId generates an election id that is monotonically increasing with time.
// Specifically, the upper 64 bits are the unix timestamp in seconds, and the
// lower 64 bits are the remaining nanoseconds. This is compatible with
// election-systems that use the same epoch-based election IDs, and in that
// case, this election ID will be guaranteed to be higher than any previous
// election ID. This is useful in tests where repeated connections need to
// acquire mastership reliably.
func TimeBasedElectionId() p4_v1.Uint128 {
	now := time.Now()
	return p4_v1.Uint128{
		High: uint64(now.Unix()),
		Low:  uint64(now.UnixNano() % 1e9),
	}
}
