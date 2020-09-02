// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewJoinIPMulticastInformationWithinUsageReport creates a new JoinIPMulticastInformationWithinUsageReport IE.
func NewJoinIPMulticastInformationWithinUsageReport(ies ...*IE) *IE {
	return newGroupedIE(JoinIPMulticastInformationWithinUsageReport, 0, ies...)
}

// JoinIPMulticastInformationWithinUsageReport returns the IEs above JoinIPMulticastInformationWithinUsageReport if the type of IE matches.
func (i *IE) JoinIPMulticastInformationWithinUsageReport() ([]*IE, error) {
	switch i.Type {
	case JoinIPMulticastInformationWithinUsageReport:
		return ParseMultiIEs(i.Payload)
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == JoinIPMulticastInformationWithinUsageReport {
				return x.JoinIPMulticastInformationWithinUsageReport()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
