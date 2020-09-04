// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateURR creates a new UpdateURR IE.
func NewUpdateURR(ies ...*IE) *IE {

	return newGroupedIE(UpdateURR, 0, ies...)
}

// UpdateURR returns the IEs above UpdateURR if the type of IE matches.
func (i *IE) UpdateURR() ([]*IE, error) {
	if i.Type != UpdateURR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
