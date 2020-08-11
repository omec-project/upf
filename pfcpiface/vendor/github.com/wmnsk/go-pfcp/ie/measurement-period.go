// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewMeasurementPeriod creates a new MeasurementPeriod IE.
//
// the period should be within the range of uint32, otherwise it overflows.
func NewMeasurementPeriod(period time.Duration) *IE {
	return newUint32ValIE(MeasurementPeriod, uint32(period.Seconds()))
}

// MeasurementPeriod returns MeasurementPeriod in time.Duration if the type of IE matches.
func (i *IE) MeasurementPeriod() (time.Duration, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MeasurementPeriod:
		return time.Duration(binary.BigEndian.Uint32(i.Payload[0:4])) * time.Second, nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MeasurementPeriod {
				return x.MeasurementPeriod()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MeasurementPeriod {
				return x.MeasurementPeriod()
			}
		}
		return 0, ErrIENotFound
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MeasurementPeriod {
				return x.MeasurementPeriod()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
