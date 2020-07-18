// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewAccessAvailabilityControlInformation creates a new AccessAvailabilityControlInformation IE.
func NewAccessAvailabilityControlInformation(info *IE) *IE {
	return newGroupedIE(AccessAvailabilityControlInformation, 0, info)
}

// AccessAvailabilityControlInformation returns the IEs above AccessAvailabilityControlInformation if the type of IE matches.
func (i *IE) AccessAvailabilityControlInformation() ([]*IE, error) {
	switch i.Type {
	case AccessAvailabilityControlInformation:
		return ParseMultiIEs(i.Payload)
	case CreateSRR:
		ies, err := i.CreateSRR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AccessAvailabilityControlInformation {
				return x.AccessAvailabilityControlInformation()
			}
		}
		return nil, ErrIENotFound
	case UpdateSRR:
		ies, err := i.UpdateSRR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AccessAvailabilityControlInformation {
				return x.AccessAvailabilityControlInformation()
			}
		}
		return nil, ErrIENotFound
	case SessionReport:
		ies, err := i.SessionReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AccessAvailabilityControlInformation {
				return x.AccessAvailabilityControlInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
