// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewPagingPolicyIndicator creates a new PagingPolicyIndicator IE.
func NewPagingPolicyIndicator(indicator uint8) *IE {
	return newUint8ValIE(PagingPolicyIndicator, indicator)
}

// PagingPolicyIndicator returns PagingPolicyIndicator in uint8 if the type of IE matches.
func (i *IE) PagingPolicyIndicator() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case PagingPolicyIndicator:
		return i.Payload[0] & 0x07, nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PagingPolicyIndicator {
				return x.PagingPolicyIndicator()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PagingPolicyIndicator {
				return x.PagingPolicyIndicator()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
