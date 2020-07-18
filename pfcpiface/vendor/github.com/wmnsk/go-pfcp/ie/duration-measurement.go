// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewDurationMeasurement creates a new DurationMeasurement IE.
//
// The period should be within the range of uint32, otherwise it overflows.
func NewDurationMeasurement(duration time.Duration) *IE {
	return newUint32ValIE(DurationMeasurement, uint32(duration.Seconds()))
}

// DurationMeasurement returns DurationMeasurement in time.Duration if the type of IE matches.
func (i *IE) DurationMeasurement() (time.Duration, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case DurationMeasurement:
		return time.Duration(binary.BigEndian.Uint32(i.Payload[0:4])) * time.Second, nil
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DurationMeasurement {
				return x.DurationMeasurement()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
