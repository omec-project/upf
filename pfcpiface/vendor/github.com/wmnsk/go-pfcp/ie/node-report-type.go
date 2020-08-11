// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewNodeReportType creates a new NodeReportType IE.
func NewNodeReportType(flags uint8) *IE {
	return newUint8ValIE(NodeReportType, flags)
}

// NodeReportType returns NodeReportType in uint8 if the type of IE matches.
func (i *IE) NodeReportType() (uint8, error) {
	if i.Type != NodeReportType {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	return i.Payload[0], nil
}

// HasUPFR reports whether an IE has UPFR bit.
func (i *IE) HasUPFR() bool {
	if i.Type != NodeReportType {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
