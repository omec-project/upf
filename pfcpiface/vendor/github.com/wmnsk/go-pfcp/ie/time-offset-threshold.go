// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewTimeOffsetThreshold creates a new TimeOffsetThreshold IE.
func NewTimeOffsetThreshold(threshold time.Duration) *IE {
	return newUint64ValIE(TimeOffsetThreshold, uint64(threshold.Nanoseconds()))
}

// TimeOffsetThreshold returns TimeOffsetThreshold in time.Duration if the type of IE matches.
func (i *IE) TimeOffsetThreshold() (time.Duration, error) {
	if len(i.Payload) < 8 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case TimeOffsetThreshold:
		return time.Duration(binary.BigEndian.Uint64(i.Payload[0:8])), nil
	case ClockDriftControlInformation:
		ies, err := i.ClockDriftControlInformation()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == TimeOffsetThreshold {
				return x.TimeOffsetThreshold()
			}
		}
		return 0, ErrIENotFound
	case ClockDriftReport:
		ies, err := i.ClockDriftReport()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == TimeOffsetThreshold {
				return x.TimeOffsetThreshold()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
