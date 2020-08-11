// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRQI creates a new RQI IE.
func NewRQI(rqi uint8) *IE {
	return newUint8ValIE(RQI, rqi)
}

// RQI returns RQI in []byte if the type of IE matches.
func (i *IE) RQI() ([]byte, error) {
	if len(i.Payload) < 1 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RQI:
		return i.Payload, nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RQI {
				return x.RQI()
			}
		}
		return nil, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RQI {
				return x.RQI()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasRQI reports whether an IE has RQI bit.
func (i *IE) HasRQI() bool {
	v, err := i.RQI()
	if err != nil {
		return false
	}

	return has1stBit(v[0])
}
