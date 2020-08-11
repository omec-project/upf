// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSRRspFlags creates a new PFCPSRRspFlags IE.
func NewPFCPSRRspFlags(flag uint8) *IE {
	return newUint8ValIE(PFCPSRRspFlags, flag)
}

// PFCPSRRspFlags returns PFCPSRRspFlags in []byte if the type of IE matches.
func (i *IE) PFCPSRRspFlags() ([]byte, error) {
	if i.Type != PFCPSRRspFlags {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}
