// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUsageReport creates a new UsageReport IE.
func NewUsageReport(typ uint16, ies ...*IE) *IE {
	return newGroupedIE(typ, 0, ies...)
}

// NewUsageReportWithinSessionModificationResponse creates a new UsageReportWithinSessionModificationResponse IE.
func NewUsageReportWithinSessionModificationResponse(ies ...*IE) *IE {
	return NewUsageReport(UsageReportWithinSessionModificationResponse, ies...)
}

// NewUsageReportWithinSessionDeletionResponse creates a new UsageReportWithinSessionDeletionResponse IE.
func NewUsageReportWithinSessionDeletionResponse(ies ...*IE) *IE {
	return NewUsageReport(UsageReportWithinSessionDeletionResponse, ies...)
}

// NewUsageReportWithinSessionReportRequest creates a new UsageReportWithinSessionReportRequest IE.
func NewUsageReportWithinSessionReportRequest(ies ...*IE) *IE {
	return NewUsageReport(UsageReportWithinSessionReportRequest, ies...)
}

// UsageReport returns the IEs above UsageReport if the type of IE matches.
func (i *IE) UsageReport() ([]*IE, error) {
	switch i.Type {
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:

		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
