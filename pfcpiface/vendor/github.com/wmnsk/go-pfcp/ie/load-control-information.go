// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewLoadControlInformation creates a new LoadControlInformation IE.
func NewLoadControlInformation(seq, metric *IE) *IE {
	return newGroupedIE(LoadControlInformation, 0, seq, metric)
}

// LoadControlInformation returns the IEs above LoadControlInformation if the type of IE matches.
func (i *IE) LoadControlInformation() ([]*IE, error) {
	if i.Type != LoadControlInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
