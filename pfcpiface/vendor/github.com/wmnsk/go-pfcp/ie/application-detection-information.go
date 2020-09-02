// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewApplicationDetectionInformation creates a new ApplicationDetectionInformation IE.
func NewApplicationDetectionInformation(ies ...*IE) *IE {
	return newGroupedIE(ApplicationDetectionInformation, 0, ies...)
}

// ApplicationDetectionInformation returns the IEs above ApplicationDetectionInformation if the type of IE matches.
func (i *IE) ApplicationDetectionInformation() ([]*IE, error) {
	switch i.Type {
	case ApplicationDetectionInformation:
		return ParseMultiIEs(i.Payload)
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ApplicationDetectionInformation {
				return x.ApplicationDetectionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
