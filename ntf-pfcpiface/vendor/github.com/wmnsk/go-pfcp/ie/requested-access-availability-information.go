// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRequestedAccessAvailabilityInformation creates a new RequestedAccessAvailabilityInformation IE.
func NewRequestedAccessAvailabilityInformation(rrca uint8) *IE {
	return newUint8ValIE(RequestedAccessAvailabilityInformation, rrca&0x01)
}

// RequestedAccessAvailabilityInformation returns RequestedAccessAvailabilityInformation in uint8 if the type of IE matches.
func (i *IE) RequestedAccessAvailabilityInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RequestedAccessAvailabilityInformation:
		return i.Payload[0], nil
	case AccessAvailabilityControlInformation:
		ies, err := i.AccessAvailabilityControlInformation()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == RequestedAccessAvailabilityInformation {
				return x.RequestedAccessAvailabilityInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasRRCA reports whether an IE has RRCA bit.
func (i *IE) HasRRCA() bool {
	v, err := i.RequestedAccessAvailabilityInformation()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
