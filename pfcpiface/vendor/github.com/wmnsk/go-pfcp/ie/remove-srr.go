// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveSRR creates a new RemoveSRR IE.
func NewRemoveSRR(srr *IE) *IE {
	return newGroupedIE(RemoveSRR, 0, srr)
}

// RemoveSRR returns the IEs above RemoveSRR if the type of IE matches.
func (i *IE) RemoveSRR() ([]*IE, error) {
	if i.Type != RemoveSRR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
