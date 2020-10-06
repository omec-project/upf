// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreatedPDR creates a new CreatedPDR IE.
func NewCreatedPDR(ies ...*IE) *IE {
	return newGroupedIE(CreatedPDR, 0, ies...)
}

// CreatedPDR returns the IEs above CreatedPDR if the type of IE matches.
func (i *IE) CreatedPDR() ([]*IE, error) {
	if i.Type != CreatedPDR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
