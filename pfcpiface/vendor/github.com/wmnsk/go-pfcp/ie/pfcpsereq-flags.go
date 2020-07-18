// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSEReqFlags creates a new PFCPSEReqFlags IE.
func NewPFCPSEReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPSEReqFlags, flag)
}

// PFCPSEReqFlags returns PFCPSEReqFlags in []byte if the type of IE matches.
func (i *IE) PFCPSEReqFlags() ([]byte, error) {
	if i.Type != PFCPSEReqFlags {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// HasRESTI reports whether an IE has RESTI bit.
func (i *IE) HasRESTI() bool {
	if i.Type != PFCPSEReqFlags {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
