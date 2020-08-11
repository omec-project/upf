// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveMAR creates a new RemoveMAR IE.
func NewRemoveMAR(marID *IE) *IE {
	return newGroupedIE(RemoveMAR, 0, marID)
}

// RemoveMAR returns the IEs above RemoveMAR if the type of IE matches.
func (i *IE) RemoveMAR() ([]*IE, error) {
	if i.Type != RemoveMAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
