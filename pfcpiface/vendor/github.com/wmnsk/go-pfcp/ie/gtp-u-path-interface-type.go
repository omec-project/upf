// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewGTPUPathInterfaceType creates a new GTPUPathInterfaceType IE.
func NewGTPUPathInterfaceType(n3, n9 int) *IE {
	return newUint8ValIE(GTPUPathInterfaceType, uint8((n3<<1)|n9))
}

// GTPUPathInterfaceType returns GTPUPathInterfaceType in uint8 if the type of IE matches.
func (i *IE) GTPUPathInterfaceType() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case GTPUPathInterfaceType:
		return i.Payload[0], nil
	case GTPUPathQoSControlInformation:
		ies, err := i.GTPUPathQoSControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == GTPUPathInterfaceType {
				return x.GTPUPathInterfaceType()
			}
		}
		return 0, ErrIENotFound
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == GTPUPathInterfaceType {
				return x.GTPUPathInterfaceType()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}

// HasN3 reports whether an IE has N3 bit.
func (i *IE) HasN3() bool {
	v, err := i.GTPUPathInterfaceType()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasN9 reports whether an IE has N9 bit.
func (i *IE) HasN9() bool {
	v, err := i.GTPUPathInterfaceType()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
