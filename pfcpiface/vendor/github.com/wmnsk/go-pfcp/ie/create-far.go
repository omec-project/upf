// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateFAR creates a new CreateFAR IE.
func NewCreateFAR(ies ...*IE) *IE {
	return newGroupedIE(CreateFAR, 0, ies...)
}

// CreateFAR returns the IEs above CreateFAR if the type of IE matches.
func (i *IE) CreateFAR() ([]*IE, error) {
	if i.Type != CreateFAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
