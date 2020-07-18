// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveBAR creates a new RemoveBAR IE.
func NewRemoveBAR(barID *IE) *IE {
	return newGroupedIE(RemoveBAR, 0, barID)
}

// RemoveBAR returns the IEs above RemoveBAR if the type of IE matches.
func (i *IE) RemoveBAR() ([]*IE, error) {
	if i.Type != RemoveBAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
