// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewDuplicatingParameters creates a new DuplicatingParameters IE.
func NewDuplicatingParameters(ies ...*IE) *IE {
	return newGroupedIE(DuplicatingParameters, 0, ies...)
}

// DuplicatingParameters returns the IEs above DuplicatingParameters if the type of IE matches.
func (i *IE) DuplicatingParameters() ([]*IE, error) {
	switch i.Type {
	case DuplicatingParameters:
		return ParseMultiIEs(i.Payload)
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == DuplicatingParameters {
				return x.DuplicatingParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
