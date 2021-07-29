// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewPFCPASReqFlags creates a new PFCPASReqFlags IE.
func NewPFCPASReqFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPASReqFlags, flag)
}

// PFCPASReqFlags returns PFCPASReqFlags in uint8 if the type of IE matches.
func (i *IE) PFCPASReqFlags() (uint8, error) {
	if i.Type != PFCPASReqFlags {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	return i.Payload[0], nil
}

// HasUUPSI reports whether an IE has UUPSI bit.
func (i *IE) HasUUPSI() bool {
	v, err := i.PFCPASReqFlags()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
