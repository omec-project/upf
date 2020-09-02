// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewMPTCPParameters creates a new MPTCPParameters IE.
func NewMPTCPParameters(ies ...*IE) *IE {
	return newGroupedIE(MPTCPParameters, 0, ies...)
}

// MPTCPParameters returns the IEs above MPTCPParameters if the type of IE matches.
func (i *IE) MPTCPParameters() ([]*IE, error) {
	if i.Type != MPTCPParameters {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
