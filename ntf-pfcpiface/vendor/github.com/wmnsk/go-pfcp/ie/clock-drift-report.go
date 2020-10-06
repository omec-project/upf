// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewClockDriftReport creates a new ClockDriftReport IE.
func NewClockDriftReport(ies ...*IE) *IE {
	return newGroupedIE(ClockDriftReport, 0, ies...)
}

// ClockDriftReport returns the IEs above ClockDriftReport if the type of IE matches.
func (i *IE) ClockDriftReport() ([]*IE, error) {
	if i.Type != ClockDriftReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
