// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewOverloadControlInformation creates a new OverloadControlInformation IE.
func NewOverloadControlInformation(ies ...*IE) *IE {
	return newGroupedIE(OverloadControlInformation, 0, ies...)
}

// OverloadControlInformation returns the IEs above OverloadControlInformation if the type of IE matches.
func (i *IE) OverloadControlInformation() ([]*IE, error) {
	if i.Type != OverloadControlInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
