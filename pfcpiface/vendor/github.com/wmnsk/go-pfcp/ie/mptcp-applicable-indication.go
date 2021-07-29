// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewMPTCPApplicableIndication creates a new MPTCPApplicableIndication IE.
func NewMPTCPApplicableIndication(flag uint8) *IE {
	return newUint8ValIE(MPTCPApplicableIndication, flag)
}

// MPTCPApplicableIndication returns MPTCPApplicableIndication in uint8 if the type of IE matches.
func (i *IE) MPTCPApplicableIndication() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MPTCPApplicableIndication:
		return i.Payload[0], nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MPTCPApplicableIndication {
				return x.MPTCPApplicableIndication()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasMAI reports whether an IE has MAI bit.
func (i *IE) HasMAI() bool {
	switch i.Type {
	case MPTCPApplicableIndication:
		if len(i.Payload) < 1 {
			return false
		}

		return has1stBit(i.Payload[0])
	default:
		return false
	}
}
