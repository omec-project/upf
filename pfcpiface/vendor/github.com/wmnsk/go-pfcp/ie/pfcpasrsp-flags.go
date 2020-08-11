// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPASRspFlags creates a new PFCPASRspFlags IE.
func NewPFCPASRspFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPASRspFlags, flag)
}

// PFCPASRspFlags returns PFCPASRspFlags in []byte if the type of IE matches.
func (i *IE) PFCPASRspFlags() ([]byte, error) {
	if i.Type != PFCPASRspFlags {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// HasPSREI reports whether an IE has PSREI bit.
func (i *IE) HasPSREI() bool {
	if i.Type != PFCPASRspFlags {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
