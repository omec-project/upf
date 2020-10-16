// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewPFCPASRspFlags creates a new PFCPASRspFlags IE.
func NewPFCPASRspFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPASRspFlags, flag)
}

// PFCPASRspFlags returns PFCPASRspFlags in uint8 if the type of IE matches.
func (i *IE) PFCPASRspFlags() (uint8, error) {
	if i.Type != PFCPASRspFlags {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	return i.Payload[0], nil
}

// HasPSREI reports whether an IE has PSREI bit.
func (i *IE) HasPSREI() bool {
	v, err := i.PFCPASRspFlags()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
