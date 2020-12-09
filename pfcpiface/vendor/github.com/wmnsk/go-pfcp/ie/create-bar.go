// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateBAR creates a new CreateBAR IE.
func NewCreateBAR(ies ...*IE) *IE {
	return newGroupedIE(CreateBAR, 0, ies...)
}

// CreateBAR returns the IEs above CreateBAR if the type of IE matches.
func (i *IE) CreateBAR() ([]*IE, error) {
	switch i.Type {
	case CreateBAR:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
