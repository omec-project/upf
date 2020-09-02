// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateFAR creates a new UpdateFAR IE.
func NewUpdateFAR(ies ...*IE) *IE {
	return newGroupedIE(UpdateFAR, 0, ies...)
}

// UpdateFAR returns the IEs above UpdateFAR if the type of IE matches.
func (i *IE) UpdateFAR() ([]*IE, error) {
	if i.Type != UpdateFAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
