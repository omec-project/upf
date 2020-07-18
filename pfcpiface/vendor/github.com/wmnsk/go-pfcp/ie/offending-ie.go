// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewOffendingIE creates a new OffendingIE IE.
func NewOffendingIE(itype uint16) *IE {
	return newUint16ValIE(OffendingIE, itype)
}

// OffendingIE returns OffendingIE in uint16 if the type of IE matches.
func (i *IE) OffendingIE() (uint16, error) {
	if i.Type != OffendingIE {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	if len(i.Payload) < 2 {
		return 0, io.ErrUnexpectedEOF
	}

	return binary.BigEndian.Uint16(i.Payload[0:2]), nil
}
