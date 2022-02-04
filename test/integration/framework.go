// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

// this file should contain all the struct defs/constants used among different test cases.

type pfcpSessionData struct {
	nbAddress    string
	ueAddress    string
	upfN3Address string

	sdfFilter string

	precedence uint32

	ulTEID uint32
	dlTEID uint32

	// QER-related fields
	sessQerID        uint32
	uplinkAppQerID   uint32
	downlinkAppQerID uint32
	sessQFI          uint8
	appQFI           uint8

	sessMBR uint64
	sessGBR uint64

	// uplink/downlink GBR/MBR is always the same
	appMBR uint64
	appGBR uint64
}
