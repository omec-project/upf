// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewApplicationIDsPFDs creates a new ApplicationIDsPFDs IE.
func NewApplicationIDsPFDs(ies ...*IE) *IE {
	return newGroupedIE(ApplicationIDsPFDs, 0, ies...)
}

// ApplicationIDsPFDs returns the IEs above ApplicationIDsPFDs if the type of IE matches.
func (i *IE) ApplicationIDsPFDs() ([]*IE, error) {
	if i.Type != ApplicationIDsPFDs {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
