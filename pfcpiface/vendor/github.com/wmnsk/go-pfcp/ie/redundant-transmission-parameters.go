// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRedundantTransmissionParameters creates a new RedundantTransmissionParameters IE.
func NewRedundantTransmissionParameters(ies ...*IE) *IE {
	return newGroupedIE(RedundantTransmissionParameters, 0, ies...)
}

// NewRedundantTransmissionParametersInPDI creates a new RedundantTransmissionParameters IE.
func NewRedundantTransmissionParametersInPDI(fteid, ni *IE) *IE {
	return newGroupedIE(RedundantTransmissionParameters, 0, fteid, ni)
}

// NewRedundantTransmissionParametersInFAR creates a new RedundantTransmissionParameters IE.
func NewRedundantTransmissionParametersInFAR(ohc, ni *IE) *IE {
	return newGroupedIE(RedundantTransmissionParameters, 0, ohc, ni)
}

// RedundantTransmissionParameters returns the IEs above RedundantTransmissionParameters if the type of IE matches.
func (i *IE) RedundantTransmissionParameters() ([]*IE, error) {
	switch i.Type {
	case RedundantTransmissionParameters:
		return ParseMultiIEs(i.Payload)
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionParameters {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionParameters {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionParameters {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionParameters {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedundantTransmissionParameters {
				return x.RedundantTransmissionParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
