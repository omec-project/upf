// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUEIPAddressPoolInformation creates a new UEIPAddressPoolInformation IE.
func NewUEIPAddressPoolInformation(id, instance *IE) *IE {
	return newGroupedIE(UEIPAddressPoolInformation, 0, id, instance)
}

// UEIPAddressPoolInformation returns the IEs above UEIPAddressPoolInformation if the type of IE matches.
func (i *IE) UEIPAddressPoolInformation() ([]*IE, error) {
	if i.Type != UEIPAddressPoolInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
