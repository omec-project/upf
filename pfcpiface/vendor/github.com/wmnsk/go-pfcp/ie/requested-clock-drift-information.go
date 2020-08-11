// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRequestedClockDriftInformation creates a new RequestedClockDriftInformation IE.
func NewRequestedClockDriftInformation(rrcr, rrto uint8) *IE {
	return newUint8ValIE(RequestedClockDriftInformation, (rrcr<<1)|rrto)
}

// RequestedClockDriftInformation returns RequestedClockDriftInformation in uint8 if the type of IE matches.
func (i *IE) RequestedClockDriftInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RequestedClockDriftInformation:
		return i.Payload[0], nil
	case ClockDriftControlInformation:
		ies, err := i.ClockDriftControlInformation()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == RequestedClockDriftInformation {
				return x.RequestedClockDriftInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasRRCR reports whether an IE has RRCR bit.
func (i *IE) HasRRCR() bool {
	if len(i.Payload) < 1 {
		return false
	}

	switch i.Type {
	case RequestedClockDriftInformation:
		return has2ndBit(i.Payload[0])
	/*
		case ClockDriftControlInformation:
			ies, err := i.ClockDriftControlInformation()
			if err != nil {
				return false
			}

			for _, x := range ies {
				if x.Type == RequestedClockDriftInformation {
					return x.HasRRCR()
				}
			}
			return false
	*/
	default:
		return false
	}
}

// HasRRTO reports whether an IE has RRTO bit.
func (i *IE) HasRRTO() bool {
	if len(i.Payload) < 1 {
		return false
	}

	switch i.Type {
	case RequestedClockDriftInformation:
		return has1stBit(i.Payload[0])
	/*
		case ClockDriftControlInformation:
			ies, err := i.ClockDriftControlInformation()
			if err != nil {
				return false
			}

			for _, x := range ies {
				if x.Type == RequestedClockDriftInformation {
					return x.HasRRTO()
				}
			}
			return false
	*/
	default:
		return false
	}
}
