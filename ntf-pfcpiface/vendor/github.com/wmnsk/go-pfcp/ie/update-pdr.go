// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdatePDR creates a new UpdatePDR IE.
func NewUpdatePDR(ies ...*IE) *IE {
	return newGroupedIE(UpdatePDR, 0, ies...)
}

// UpdatePDR returns the IEs above UpdatePDR if the type of IE matches.
func (i *IE) UpdatePDR() ([]*IE, error) {
	if i.Type != UpdatePDR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
