// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPortManagementInformationForTSC creates a new PortManagementInformationForTSC IE.
func NewPortManagementInformationForTSC(typ uint16, info *IE) *IE {
	return newGroupedIE(typ, 0, info)
}

// NewPortManagementInformationForTSCWithinSessionModificationRequest creates a new PortManagementInformationForTSCWithinSessionModificationRequest IE.
func NewPortManagementInformationForTSCWithinSessionModificationRequest(info *IE) *IE {
	return newGroupedIE(PortManagementInformationForTSCWithinSessionModificationRequest, 0, info)
}

// NewPortManagementInformationForTSCWithinSessionModificationResponse creates a new PortManagementInformationForTSCWithinSessionModificationResponse IE.
func NewPortManagementInformationForTSCWithinSessionModificationResponse(info *IE) *IE {
	return newGroupedIE(PortManagementInformationForTSCWithinSessionModificationResponse, 0, info)
}

// NewPortManagementInformationForTSCWithinSessionReportRequest creates a new PortManagementInformationForTSCWithinSessionReportRequest IE.
func NewPortManagementInformationForTSCWithinSessionReportRequest(info *IE) *IE {
	return newGroupedIE(PortManagementInformationForTSCWithinSessionReportRequest, 0, info)
}

// PortManagementInformationForTSC returns the IEs above PortManagementInformationForTSC if the type of IE matches.
func (i *IE) PortManagementInformationForTSC() ([]*IE, error) {
	switch i.Type {
	case PortManagementInformationForTSCWithinSessionModificationRequest,
		PortManagementInformationForTSCWithinSessionModificationResponse,
		PortManagementInformationForTSCWithinSessionReportRequest:

		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
