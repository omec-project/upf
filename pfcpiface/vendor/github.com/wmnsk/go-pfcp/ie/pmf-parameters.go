// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPMFParameters creates a new PMFParameters IE.
func NewPMFParameters(info *IE) *IE {
	return newGroupedIE(PMFParameters, 0, info)
}

// PMFParameters returns the IEs above PMFParameters if the type of IE matches.
func (i *IE) PMFParameters() ([]*IE, error) {
	switch i.Type {
	case PMFParameters:
		return ParseMultiIEs(i.Payload)
	case ATSSSControlParameters:
		ies, err := i.ATSSSControlParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PMFParameters {
				return x.PMFParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
