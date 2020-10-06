// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewATSSSControlParameters creates a new ATSSSControlParameters IE.
func NewATSSSControlParameters(ies ...*IE) *IE {
	return newGroupedIE(ATSSSControlParameters, 0, ies...)
}

// ATSSSControlParameters returns the IEs above ATSSSControlParameters if the type of IE matches.
func (i *IE) ATSSSControlParameters() ([]*IE, error) {
	if i.Type != ATSSSControlParameters {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
