// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveTrafficEndpoint creates a new RemoveTrafficEndpoint IE.
func NewRemoveTrafficEndpoint(id *IE) *IE {
	return newGroupedIE(RemoveTrafficEndpoint, 0, id)
}

// RemoveTrafficEndpoint returns the IEs above RemoveTrafficEndpoint if the type of IE matches.
func (i *IE) RemoveTrafficEndpoint() ([]*IE, error) {
	if i.Type != RemoveTrafficEndpoint {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
