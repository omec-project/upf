// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewGTPUPathQoSControlInformation creates a new GTPUPathQoSControlInformation IE.
func NewGTPUPathQoSControlInformation(ies ...*IE) *IE {
	return newGroupedIE(GTPUPathQoSControlInformation, 0, ies...)
}

// GTPUPathQoSControlInformation returns the IEs above GTPUPathQoSControlInformation if the type of IE matches.
func (i *IE) GTPUPathQoSControlInformation() ([]*IE, error) {
	if i.Type != GTPUPathQoSControlInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
