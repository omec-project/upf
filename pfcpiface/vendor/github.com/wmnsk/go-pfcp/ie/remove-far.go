// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveFAR creates a new RemoveFAR IE.
func NewRemoveFAR(pdr *IE) *IE {
	return newGroupedIE(RemoveFAR, 0, pdr)
}

// RemoveFAR returns the IEs above RemoveFAR if the type of IE matches.
func (i *IE) RemoveFAR() ([]*IE, error) {
	if i.Type != RemoveFAR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
