// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewAccessAvailabilityReport creates a new AccessAvailabilityReport IE.
func NewAccessAvailabilityReport(info *IE) *IE {
	return newGroupedIE(AccessAvailabilityReport, 0, info)
}

// AccessAvailabilityReport returns the IEs above AccessAvailabilityReport if the type of IE matches.
func (i *IE) AccessAvailabilityReport() ([]*IE, error) {
	if i.Type != AccessAvailabilityReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
