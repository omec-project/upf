// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSEReqFlags creates a new PFCPSEReqFlags IE.
func NewPFCPSEReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPSEReqFlags, flag)
}

// PFCPSEReqFlags returns PFCPSEReqFlags in uint8 if the type of IE matches.
func (i *IE) PFCPSEReqFlags() (uint8, error) {
	if i.Type != PFCPSEReqFlags {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload[0], nil
}

// HasRESTI reports whether an IE has RESTI bit.
func (i *IE) HasRESTI() bool {
	v, err := i.PFCPSEReqFlags()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
