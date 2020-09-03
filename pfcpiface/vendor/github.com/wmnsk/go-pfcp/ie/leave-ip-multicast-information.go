// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewLeaveIPMulticastInformationWithinUsageReport creates a new LeaveIPMulticastInformationWithinUsageReport IE.
func NewLeaveIPMulticastInformationWithinUsageReport(ies ...*IE) *IE {
	return newGroupedIE(LeaveIPMulticastInformationWithinUsageReport, 0, ies...)
}

// LeaveIPMulticastInformationWithinUsageReport returns the IEs above LeaveIPMulticastInformationWithinUsageReport if the type of IE matches.
func (i *IE) LeaveIPMulticastInformationWithinUsageReport() ([]*IE, error) {
	switch i.Type {
	case LeaveIPMulticastInformationWithinUsageReport:
		return ParseMultiIEs(i.Payload)
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == LeaveIPMulticastInformationWithinUsageReport {
				return x.LeaveIPMulticastInformationWithinUsageReport()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
