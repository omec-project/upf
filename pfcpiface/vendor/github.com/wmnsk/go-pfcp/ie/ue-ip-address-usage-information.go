// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUEIPAddressUsageInformation creates a new UEIPAddressUsageInformation IE.
func NewUEIPAddressUsageInformation(ies ...*IE) *IE {
	return newGroupedIE(UEIPAddressUsageInformation, 0, ies...)
}

// UEIPAddressUsageInformation returns the IEs above UEIPAddressUsageInformation if the type of IE matches.
func (i *IE) UEIPAddressUsageInformation() ([]*IE, error) {
	switch i.Type {
	case UEIPAddressUsageInformation:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
