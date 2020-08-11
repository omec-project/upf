// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemovePDR creates a new RemovePDR IE.
func NewRemovePDR(pdr *IE) *IE {
	return newGroupedIE(RemovePDR, 0, pdr)
}

// RemovePDR returns the IEs above RemovePDR if the type of IE matches.
func (i *IE) RemovePDR() ([]*IE, error) {
	if i.Type != RemovePDR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
