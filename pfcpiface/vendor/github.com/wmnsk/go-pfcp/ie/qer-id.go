// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewQERID creates a new QERID IE.
func NewQERID(id uint32) *IE {
	return newUint32ValIE(QERID, id)
}

// QERID returns QERID in uint32 if the type of IE matches.
func (i *IE) QERID() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case QERID:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	case RemoveQER:
		ies, err := i.RemoveQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	case PacketRateStatusReport:
		ies, err := i.PacketRateStatusReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERID {
				return x.QERID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
