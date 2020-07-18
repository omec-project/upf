// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateTrafficEndpoint creates a new UpdateTrafficEndpoint IE.
func NewUpdateTrafficEndpoint(id, fteid, ni, rtp, ueIP, route, routing, v6route, qfi *IE) *IE {
	return newGroupedIE(UpdateTrafficEndpoint, 0, id, fteid, ni, rtp, ueIP, route, routing, v6route, qfi)
}

// UpdateTrafficEndpoint returns the IEs above UpdateTrafficEndpoint if the type of IE matches.
func (i *IE) UpdateTrafficEndpoint() ([]*IE, error) {
	if i.Type != UpdateTrafficEndpoint {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
