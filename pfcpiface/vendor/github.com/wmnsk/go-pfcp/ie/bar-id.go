// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewBARID creates a new BARID IE.
func NewBARID(id uint8) *IE {
	return newUint8ValIE(BARID, id)
}

// BARID returns BARID in uint8 if the type of IE matches.
func (i *IE) BARID() (uint8, error) {
	switch i.Type {
	case BARID:
		if len(i.Payload) < 1 {
			return 0, io.ErrUnexpectedEOF
		}

		return i.Payload[0], nil
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	case QueryURR:
		ies, err := i.QueryURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	case CreateBAR:
		ies, err := i.CreateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateBARWithinSessionReportResponse,
		UpdateBARWithinSessionModificationRequest:
		ies, err := i.UpdateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	case RemoveBAR:
		ies, err := i.RemoveBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == BARID {
				return x.BARID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
