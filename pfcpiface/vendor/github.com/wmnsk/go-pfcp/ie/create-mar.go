// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateMAR creates a new CreateMAR IE.
func NewCreateMAR(ies ...*IE) *IE {
	return newGroupedIE(CreateMAR, 0, ies...)
}

// CreateMAR returns the IEs above CreateMAR if the type of IE matches.
func (i *IE) CreateMAR() ([]*IE, error) {
	if i.Type != CreateMAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
