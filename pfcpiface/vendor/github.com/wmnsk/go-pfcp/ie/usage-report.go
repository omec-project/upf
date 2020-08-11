// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUsageReport creates a new UsageReport IE.
func NewUsageReport(typ uint16, ies ...*IE) *IE {
	return newGroupedIE(typ, 0, ies...)
}

// NewUsageReportWithinSessionModificationResponse creates a new UsageReportWithinSessionModificationResponse IE.
func NewUsageReportWithinSessionModificationResponse(urr, seq, trigger, start, end, vol, dur, firstPkt, lastPkt, usage, query, eth *IE) *IE {
	return NewUsageReport(UsageReportWithinSessionModificationResponse, urr, seq, trigger, start, end, vol, dur, firstPkt, lastPkt, usage, query, eth)
}

// NewUsageReportWithinSessionDeletionResponse creates a new UsageReportWithinSessionDeletionResponse IE.
func NewUsageReportWithinSessionDeletionResponse(urr, seq, trigger, start, end, vol, dur, firstPkt, lastPkt, usage, eth *IE) *IE {
	return NewUsageReport(UsageReportWithinSessionDeletionResponse, urr, seq, trigger, start, end, vol, dur, firstPkt, lastPkt, usage, eth)
}

// NewUsageReportWithinSessionReportRequest creates a new UsageReportWithinSessionReportRequest IE.
func NewUsageReportWithinSessionReportRequest(urr, seq, trigger, start, end, vol, dur, app, ip, firstPkt, lastPkt, usage, query, ts, eth, join, leave *IE) *IE {
	return NewUsageReport(UsageReportWithinSessionReportRequest, urr, seq, trigger, start, end, vol, dur, app, ip, firstPkt, lastPkt, usage, query, ts, eth, join, leave)
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
