// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewAdditionalMonitoringTime creates a new AdditionalMonitoringTime IE.
func NewAdditionalMonitoringTime(ies ...*IE) *IE {
	return newGroupedIE(AdditionalMonitoringTime, 0, ies...)
}

// AdditionalMonitoringTime returns the IEs above AdditionalMonitoringTime if the type of IE matches.
func (i *IE) AdditionalMonitoringTime() ([]*IE, error) {
	switch i.Type {
	case AdditionalMonitoringTime:
		return ParseMultiIEs(i.Payload)
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AdditionalMonitoringTime {
				return x.AdditionalMonitoringTime()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AdditionalMonitoringTime {
				return x.AdditionalMonitoringTime()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
