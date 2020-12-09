// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewProvideATSSSControlInformation creates a new ProvideATSSSControlInformation IE.
func NewProvideATSSSControlInformation(ies ...*IE) *IE {
	return newGroupedIE(ProvideATSSSControlInformation, 0, ies...)
}

// ProvideATSSSControlInformation returns the IEs above ProvideATSSSControlInformation if the type of IE matches.
func (i *IE) ProvideATSSSControlInformation() ([]*IE, error) {
	if i.Type != ProvideATSSSControlInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
