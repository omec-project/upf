// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveURR creates a new RemoveURR IE.
func NewRemoveURR(urr *IE) *IE {
	return newGroupedIE(RemoveURR, 0, urr)
}

// RemoveURR returns the IEs above RemoveURR if the type of IE matches.
func (i *IE) RemoveURR() ([]*IE, error) {
	if i.Type != RemoveURR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
