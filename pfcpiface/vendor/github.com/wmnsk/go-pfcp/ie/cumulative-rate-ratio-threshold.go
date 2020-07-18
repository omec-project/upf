// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewCumulativeRateRatioThreshold creates a new CumulativeRateRatioThreshold IE.
func NewCumulativeRateRatioThreshold(threshold uint32) *IE {
	return newUint32ValIE(CumulativeRateRatioThreshold, threshold)
}

// CumulativeRateRatioThreshold returns CumulativeRateRatioThreshold in uint32 if the type of IE matches.
func (i *IE) CumulativeRateRatioThreshold() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case CumulativeRateRatioThreshold:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case ClockDriftControlInformation:
		ies, err := i.ClockDriftControlInformation()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == CumulativeRateRatioThreshold {
				return x.CumulativeRateRatioThreshold()
			}
		}
		return 0, ErrIENotFound
	case ClockDriftReport:
		ies, err := i.ClockDriftReport()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == CumulativeRateRatioThreshold {
				return x.CumulativeRateRatioThreshold()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
