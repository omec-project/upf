// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateDuplicatingParameters creates a new UpdateDuplicatingParameters IE.
func NewUpdateDuplicatingParameters(ies ...*IE) *IE {
	return newGroupedIE(UpdateDuplicatingParameters, 0, ies...)
}

// UpdateDuplicatingParameters returns the IEs above UpdateDuplicatingParameters if the type of IE matches.
func (i *IE) UpdateDuplicatingParameters() ([]*IE, error) {
	switch i.Type {
	case UpdateDuplicatingParameters:
		return ParseMultiIEs(i.Payload)
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UpdateDuplicatingParameters {
				return x.UpdateDuplicatingParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
