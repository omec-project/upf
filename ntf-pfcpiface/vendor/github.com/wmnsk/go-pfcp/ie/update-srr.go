// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateSRR creates a new UpdateSRR IE.
func NewUpdateSRR(ies ...*IE) *IE {
	return newGroupedIE(UpdateSRR, 0, ies...)
}

// UpdateSRR returns the IEs above UpdateSRR if the type of IE matches.
func (i *IE) UpdateSRR() ([]*IE, error) {
	switch i.Type {
	case UpdateSRR:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
