// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewQueryURR creates a new QueryURR IE.
func NewQueryURR(urrID *IE) *IE {
	return newGroupedIE(QueryURR, 0, urrID)
}

// QueryURR returns the IEs above QueryURR if the type of IE matches.
func (i *IE) QueryURR() ([]*IE, error) {
	if i.Type != QueryURR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
