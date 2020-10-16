// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPAUReqFlags creates a new PFCPAUReqFlags IE.
func NewPFCPAUReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPAUReqFlags, flag)
}

// PFCPAUReqFlags returns PFCPAUReqFlags in uint8 if the type of IE matches.
func (i *IE) PFCPAUReqFlags() (uint8, error) {
	if i.Type != PFCPAUReqFlags {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload[0], nil
}

// HasPARPS reports whether an IE has PARPS bit.
func (i *IE) HasPARPS() bool {
	v, err := i.PFCPAUReqFlags()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
