// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRQI creates a new RQI IE.
func NewRQI(rqi uint8) *IE {
	return newUint8ValIE(RQI, rqi)
}

// RQI returns RQI in uint8 if the type of IE matches.
func (i *IE) RQI() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RQI:
		return i.Payload[0], nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == RQI {
				return x.RQI()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == RQI {
				return x.RQI()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasRQI reports whether an IE has RQI bit.
func (i *IE) HasRQI() bool {
	v, err := i.RQI()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
