// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// PDNType definitions.
const (
	_               uint8 = 0
	PDNTypeIPv4     uint8 = 1
	PDNTypeIPv6     uint8 = 2
	PDNTypeIPv4v6   uint8 = 3
	PDNTypeNonIP    uint8 = 4
	PDNTypeEthernet uint8 = 5
)

// NewPDNType creates a new PDNType IE.
func NewPDNType(typ uint8) *IE {
	return newUint8ValIE(PDNType, typ)
}

// PDNType returns PDNType in uint8 if the type of IE matches.
func (i *IE) PDNType() (uint8, error) {
	if i.Type != PDNType {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	return i.Payload[0], nil
}
