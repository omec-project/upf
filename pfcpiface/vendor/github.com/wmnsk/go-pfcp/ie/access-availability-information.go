// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// AccessType definitions.
const (
	AccessType3GPP    uint8 = 0
	AccessTypeNon3GPP uint8 = 1
)

// AvailabilityStatus definitions.
const (
	AvailabilityStatusAccessHasBecomeUnavaiable uint8 = 0
	AvailabilityStatusAccessHasBecomeAvaiable   uint8 = 1
)

// NewAccessAvailabilityInformation creates a new AccessAvailabilityInformation IE.
func NewAccessAvailabilityInformation(status, atype uint8) *IE {
	return newUint8ValIE(AccessAvailabilityInformation, ((status&0x03)<<2)|atype&0x03)
}

// AccessAvailabilityInformation returns AccessAvailabilityInformation in uint8 if the type of IE matches.
func (i *IE) AccessAvailabilityInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case AccessAvailabilityInformation:
		return i.Payload[0], nil
	case AccessAvailabilityReport:
		ies, err := i.AccessAvailabilityReport()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == AccessAvailabilityInformation {
				return x.AccessAvailabilityInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// AvailabilityStatus returns AvailabilityStatus in uint8 if the type of IE matches.
func (i *IE) AvailabilityStatus() (uint8, error) {
	v, err := i.AccessAvailabilityInformation()
	if err != nil {
		return 0, err
	}

	return (v >> 2) & 0x03, nil
}

// AccessType returns AccessType in uint8 if the type of IE matches.
func (i *IE) AccessType() (uint8, error) {
	v, err := i.AccessAvailabilityInformation()
	if err != nil {
		return 0, err
	}

	return v & 0x03, nil
}
