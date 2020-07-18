// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewCumulativeRateRatioMeasurement creates a new CumulativeRateRatioMeasurement IE.
func NewCumulativeRateRatioMeasurement(measurement uint32) *IE {
	return newUint32ValIE(CumulativeRateRatioMeasurement, measurement)
}

// CumulativeRateRatioMeasurement returns CumulativeRateRatioMeasurement in uint32 if the type of IE matches.
func (i *IE) CumulativeRateRatioMeasurement() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case CumulativeRateRatioMeasurement:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
