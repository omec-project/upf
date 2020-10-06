// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewEthernetTrafficInformation creates a new EthernetTrafficInformation IE.
func NewEthernetTrafficInformation(ies ...*IE) *IE {
	return newGroupedIE(EthernetTrafficInformation, 0, ies...)
}

// EthernetTrafficInformation returns the IEs above EthernetTrafficInformation if the type of IE matches.
func (i *IE) EthernetTrafficInformation() ([]*IE, error) {
	switch i.Type {
	case EthernetTrafficInformation:
		return ParseMultiIEs(i.Payload)
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetTrafficInformation {
				return x.EthernetTrafficInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
