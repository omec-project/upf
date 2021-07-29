// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRedundantTransmissionForwardingParameters creates a new RedundantTransmissionForwardingParameters IE.
func NewRedundantTransmissionForwardingParameters(ies ...*IE) *IE {
	return newGroupedIE(RedundantTransmissionForwardingParameters, 0, ies...)
}

// RedundantTransmissionForwardingParameters returns the IEs above RedundantTransmissionForwardingParameters if the type of IE matches.
func (i *IE) RedundantTransmissionForwardingParameters() ([]*IE, error) {
	switch i.Type {
	case RedundantTransmissionForwardingParameters:
		return ParseMultiIEs(i.Payload)
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionForwardingParameters {
				return x.RedundantTransmissionForwardingParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
