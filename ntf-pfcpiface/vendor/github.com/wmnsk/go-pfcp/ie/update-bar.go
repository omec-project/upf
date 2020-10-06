// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateBAR creates a new UpdateBAR IE.
func NewUpdateBAR(typ uint16, ies ...*IE) *IE {
	return newGroupedIE(typ, 0, ies...)
}

// NewUpdateBARWithinSessionModificationRequest creates a new UpdateBARWithinSessionModificationRequest IE.
func NewUpdateBARWithinSessionModificationRequest(ies ...*IE) *IE {
	return NewUpdateBAR(UpdateBARWithinSessionModificationRequest, ies...)
}

// NewUpdateBARWithinSessionReportResponse creates a new UpdateBARWithinSessionReportResponse IE.
func NewUpdateBARWithinSessionReportResponse(bar, delay, duration, dlCount, bufCount *IE) *IE {
	return NewUpdateBAR(UpdateBARWithinSessionReportResponse, bar, delay, duration, dlCount, bufCount)
}

// UpdateBAR returns the IEs above UpdateBAR if the type of IE matches.
func (i *IE) UpdateBAR() ([]*IE, error) {
	switch i.Type {
	case UpdateBARWithinSessionModificationRequest,
		UpdateBARWithinSessionReportResponse:

		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
