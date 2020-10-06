// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateMAR creates a new UpdateMAR IE.
func NewUpdateMAR(ies ...*IE) *IE {
	return newGroupedIE(UpdateMAR, 0, ies...)
}

// UpdateMAR returns the IEs above UpdateMAR if the type of IE matches.
func (i *IE) UpdateMAR() ([]*IE, error) {
	if i.Type != UpdateMAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
