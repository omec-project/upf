// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPortManagementInformationContainer creates a new PortManagementInformationContainer IE.
func NewPortManagementInformationContainer(info string) *IE {
	return newStringIE(PortManagementInformationContainer, info)
}

// PortManagementInformationContainer returns PortManagementInformationContainer in string if the type of IE matches.
func (i *IE) PortManagementInformationContainer() (string, error) {
	switch i.Type {
	case PortManagementInformationContainer:
		return string(i.Payload), nil
	case PortManagementInformationForTSCWithinSessionModificationRequest,
		PortManagementInformationForTSCWithinSessionModificationResponse,
		PortManagementInformationForTSCWithinSessionReportRequest:
		ies, err := i.PortManagementInformationForTSC()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == PortManagementInformationContainer {
				return x.PortManagementInformationContainer()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
