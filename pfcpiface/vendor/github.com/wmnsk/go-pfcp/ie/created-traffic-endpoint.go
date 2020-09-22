// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreatedTrafficEndpoint creates a new CreatedTrafficEndpoint IE.
func NewCreatedTrafficEndpoint(ies ...*IE) *IE {
	return newGroupedIE(CreatedTrafficEndpoint, 0, ies...)
}

// CreatedTrafficEndpoint returns the IEs above CreatedTrafficEndpoint if the type of IE matches.
func (i *IE) CreatedTrafficEndpoint() ([]*IE, error) {
	if i.Type != CreatedTrafficEndpoint {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}

// LocalFTEID returns FTEID that is found first in a grouped IE in structured format
// if the type of IE matches.
//
// This can only be used on the grouped IEs that may have multiple Local F-TEID IEs.
func (i *IE) LocalFTEID() (*FTEIDFields, error) {
	switch i.Type {
	case CreatedTrafficEndpoint:
		ies, err := i.CreatedTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// LocalFTEIDN returns FTEID that is found Nth in a grouped IE in structured format
// if the type of IE matches.
//
// This can only be used on the grouped IEs that may have multiple Local F-TEID IEs.
func (i *IE) LocalFTEIDN(n int) (*FTEIDFields, error) {
	if n < 1 {
		return nil, ErrIENotFound
	}

	switch i.Type {
	case CreatedTrafficEndpoint: // has two F-TEID
		ies, err := i.CreatedTrafficEndpoint()
		if err != nil {
			return nil, err
		}

		c := 0
		for _, x := range ies {
			if x.Type == FTEID {
				c++
				if c == n {
					return x.FTEID()
				}
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
