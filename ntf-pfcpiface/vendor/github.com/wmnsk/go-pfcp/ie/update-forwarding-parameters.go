// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateForwardingParameters creates a new UpdateForwardingParameters IE.
func NewUpdateForwardingParameters(ies ...*IE) *IE {
	return newGroupedIE(UpdateForwardingParameters, 0, ies...)
}

// UpdateForwardingParameters returns the IEs above UpdateForwardingParameters if the type of IE matches.
func (i *IE) UpdateForwardingParameters() ([]*IE, error) {
	switch i.Type {
	case UpdateForwardingParameters:
		return ParseMultiIEs(i.Payload)
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UpdateForwardingParameters {
				return x.UpdateForwardingParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
