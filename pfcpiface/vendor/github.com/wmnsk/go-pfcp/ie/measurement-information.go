// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewMeasurementInformation creates a new MeasurementInformation IE.
func NewMeasurementInformation(flags uint8) *IE {
	return newUint8ValIE(MeasurementInformation, flags)
}

// MeasurementInformation returns MeasurementInformation in uint8 if the type of IE matches.
func (i *IE) MeasurementInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MeasurementInformation:
		return i.Payload[0], nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MeasurementInformation {
				return x.MeasurementInformation()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MeasurementInformation {
				return x.MeasurementInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasISTM reports whether an IE has ISTM bit.
func (i *IE) HasISTM() bool {
	if i.Type != MeasurementInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has4thBit(i.Payload[0])
}

// HasRADI reports whether an IE has RADI bit.
func (i *IE) HasRADI() bool {
	if i.Type != MeasurementInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has3rdBit(i.Payload[0])
}

// HasINAM reports whether an IE has INAM bit.
func (i *IE) HasINAM() bool {
	if i.Type != MeasurementInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has2ndBit(i.Payload[0])
}

// HasMBQE reports whether an IE has MBQE bit.
func (i *IE) HasMBQE() bool {
	if i.Type != MeasurementInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
