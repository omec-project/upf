// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
)

// NewNumberOfReports creates a new NumberOfReports IE.
func NewNumberOfReports(num uint16) *IE {
	return newUint16ValIE(NumberOfReports, num)
}

// NumberOfReports returns NumberOfReports in uint16 if the type of IE matches.
func (i *IE) NumberOfReports() (uint16, error) {
	if len(i.Payload) < 2 {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	switch i.Type {
	case NumberOfReports:
		return binary.BigEndian.Uint16(i.Payload[0:2]), nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == NumberOfReports {
				return x.NumberOfReports()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == NumberOfReports {
				return x.NumberOfReports()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
