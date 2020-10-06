// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdatedPDR creates a new UpdatedPDR IE.
func NewUpdatedPDR(ies ...*IE) *IE {
	return newGroupedIE(UpdatedPDR, 0, ies...)
}

// UpdatedPDR returns the IEs above UpdatedPDR if the type of IE matches.
func (i *IE) UpdatedPDR() ([]*IE, error) {
	if i.Type != UpdatedPDR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
