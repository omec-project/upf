// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewForwardingParameters creates a new ForwardingParameters IE.
func NewForwardingParameters(di, ni, redi, ohc, tlm, fp, he, ltei, prx, dit, dnai *IE) *IE {
	return newGroupedIE(ForwardingParameters, 0, di, ni, redi, ohc, tlm, fp, he, ltei, prx, dit, dnai)
}

// ForwardingParameters returns the IEs above ForwardingParameters if the type of IE matches.
func (i *IE) ForwardingParameters() ([]*IE, error) {
	switch i.Type {
	case ForwardingParameters:
		return ParseMultiIEs(i.Payload)
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ForwardingParameters {
				return x.ForwardingParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
