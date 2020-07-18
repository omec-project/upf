// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewQERControlIndications creates a new QERControlIndications IE.
func NewQERControlIndications(nord, mode, rcsr int) *IE {
	return newUint8ValIE(QERControlIndications, uint8((nord<<2)|(mode<<1)|(rcsr)))
}

// QERControlIndications returns QERControlIndications in uint8 if the type of IE matches.
func (i *IE) QERControlIndications() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case QERControlIndications:
		return i.Payload[0], nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERControlIndications {
				return x.QERControlIndications()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERControlIndications {
				return x.QERControlIndications()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}

// HasNORD reports whether an IE has NORD bit.
func (i *IE) HasNORD() bool {
	v, err := i.QERControlIndications()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasMODE reports whether an IE has MODE bit.
func (i *IE) HasMODE() bool {
	v, err := i.QERControlIndications()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasRCSR reports whether an IE has RCSR bit.
func (i *IE) HasRCSR() bool {
	v, err := i.QERControlIndications()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
