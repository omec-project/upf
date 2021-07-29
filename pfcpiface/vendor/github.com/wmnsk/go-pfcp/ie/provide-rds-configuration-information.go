// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewProvideRDSConfigurationInformation creates a new ProvideRDSConfigurationInformation IE.
func NewProvideRDSConfigurationInformation(ies ...*IE) *IE {
	return newGroupedIE(ProvideRDSConfigurationInformation, 0, ies...)
}

// ProvideRDSConfigurationInformation returns the IEs above ProvideRDSConfigurationInformation if the type of IE matches.
func (i *IE) ProvideRDSConfigurationInformation() ([]*IE, error) {
	if i.Type != ProvideRDSConfigurationInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
