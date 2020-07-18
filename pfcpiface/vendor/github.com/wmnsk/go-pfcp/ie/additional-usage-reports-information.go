// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewAdditionalUsageReportsInformation creates a new AdditionalUsageReportsInformation IE.
func NewAdditionalUsageReportsInformation(num uint16) *IE {
	if num == 0 {
		return newUint16ValIE(AdditionalUsageReportsInformation, num)
	}
	return newUint16ValIE(AdditionalUsageReportsInformation, (num | 0x8000))
}

// AdditionalUsageReportsInformation returns AdditionalUsageReportsInformation in uint16 if the type of IE matches.
func (i *IE) AdditionalUsageReportsInformation() (uint16, error) {
	if i.Type != AdditionalUsageReportsInformation {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 2 {
		return 0, io.ErrUnexpectedEOF
	}

	num := binary.BigEndian.Uint16(i.Payload[0:2])
	return num & 0x7fff, nil
}

// HasAURI reports whether an IE has AURI bit.
func (i *IE) HasAURI() bool {
	if i.Type != AdditionalUsageReportsInformation {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has8thBit(i.Payload[0])
}
