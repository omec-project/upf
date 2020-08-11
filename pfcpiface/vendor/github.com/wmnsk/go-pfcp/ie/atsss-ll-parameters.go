// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewATSSSLLParameters creates a new ATSSSLLParameters IE.
func NewATSSSLLParameters(info *IE) *IE {
	return newGroupedIE(ATSSSLLParameters, 0, info)
}

// ATSSSLLParameters returns the IEs above ATSSSLLParameters if the type of IE matches.
func (i *IE) ATSSSLLParameters() ([]*IE, error) {
	switch i.Type {
	case ATSSSLLParameters:
		return ParseMultiIEs(i.Payload)
	case ATSSSControlParameters:
		ies, err := i.ATSSSControlParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ATSSSLLParameters {
				return x.ATSSSLLParameters()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
