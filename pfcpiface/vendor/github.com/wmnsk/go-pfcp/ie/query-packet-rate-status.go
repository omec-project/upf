// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewQueryPacketRateStatusWithinSessionModificationRequest creates a new QueryPacketRateStatusWithinSessionModificationRequest IE.
func NewQueryPacketRateStatusWithinSessionModificationRequest(ies ...*IE) *IE {
	return newGroupedIE(QueryPacketRateStatusWithinSessionModificationRequest, 0, ies...)
}

// QueryPacketRateStatus returns the IEs above QueryPacketRateStatus if the type of IE matches.
func (i *IE) QueryPacketRateStatus() ([]*IE, error) {
	if i.Type != QueryPacketRateStatusWithinSessionModificationRequest {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
