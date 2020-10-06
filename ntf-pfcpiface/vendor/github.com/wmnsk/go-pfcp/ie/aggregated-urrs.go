// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewAggregatedURRs creates a new AggregatedURRs IE.
func NewAggregatedURRs(ies ...*IE) *IE {
	return newGroupedIE(AggregatedURRs, 0, ies...)
}

// AggregatedURRs returns the IEs above AggregatedURRs if the type of IE matches.
func (i *IE) AggregatedURRs() ([]*IE, error) {
	switch i.Type {
	case AggregatedURRs:
		return ParseMultiIEs(i.Payload)
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AggregatedURRs {
				return x.AggregatedURRs()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == AggregatedURRs {
				return x.AggregatedURRs()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
