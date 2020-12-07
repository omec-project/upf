// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSRReqFlags creates a new PFCPSRReqFlags IE.
func NewPFCPSRReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPSRReqFlags, flag)
}

// PFCPSRReqFlags returns PFCPSRReqFlags in uint8 if the type of IE matches.
func (i *IE) PFCPSRReqFlags() (uint8, error) {
	if i.Type != PFCPSRReqFlags {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload[0], nil
}

// HasPSDBU reports whether an IE has PSDBU bit.
func (i *IE) HasPSDBU() bool {
	v, err := i.PFCPSRReqFlags()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
