// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPAUReqFlags creates a new PFCPAUReqFlags IE.
func NewPFCPAUReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPAUReqFlags, flag)
}

// PFCPAUReqFlags returns PFCPAUReqFlags in []byte if the type of IE matches.
func (i *IE) PFCPAUReqFlags() ([]byte, error) {
	if i.Type != PFCPAUReqFlags {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// HasPARPS reports whether an IE has PARPS bit.
func (i *IE) HasPARPS() bool {
	if i.Type != PFCPAUReqFlags {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
