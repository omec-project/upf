// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewClockDriftControlInformation creates a new ClockDriftControlInformation IE.
func NewClockDriftControlInformation(ies ...*IE) *IE {
	return newGroupedIE(ClockDriftControlInformation, 0, ies...)
}

// ClockDriftControlInformation returns the IEs above ClockDriftControlInformation if the type of IE matches.
func (i *IE) ClockDriftControlInformation() ([]*IE, error) {
	if i.Type != ClockDriftControlInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
