// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSRReqFlags creates a new PFCPSRReqFlags IE.
func NewPFCPSRReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPSRReqFlags, flag)
}

// PFCPSRReqFlags returns PFCPSRReqFlags in []byte if the type of IE matches.
func (i *IE) PFCPSRReqFlags() ([]byte, error) {
	if i.Type != PFCPSRReqFlags {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// HasPSDBU reports whether an IE has PSDBU bit.
func (i *IE) HasPSDBU() bool {
	if i.Type != PFCPSRReqFlags {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
