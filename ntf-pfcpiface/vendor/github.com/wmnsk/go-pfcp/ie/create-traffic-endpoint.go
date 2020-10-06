// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateTrafficEndpoint creates a new CreateTrafficEndpoint IE.
func NewCreateTrafficEndpoint(ies ...*IE) *IE {
	return newGroupedIE(CreateTrafficEndpoint, 0, ies...)
}

// CreateTrafficEndpoint returns the IEs above CreateTrafficEndpoint if the type of IE matches.
func (i *IE) CreateTrafficEndpoint() ([]*IE, error) {
	if i.Type != CreateTrafficEndpoint {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
