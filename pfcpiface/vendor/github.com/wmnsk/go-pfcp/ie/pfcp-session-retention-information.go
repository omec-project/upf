// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPSessionRetentionInformation creates a new PFCPSessionRetentionInformation IE.
func NewPFCPSessionRetentionInformation(cpIP *IE) *IE {
	return newGroupedIE(PFCPSessionRetentionInformation, 0, cpIP)
}

// PFCPSessionRetentionInformation returns the IEs above PFCPSessionRetentionInformation if the type of IE matches.
func (i *IE) PFCPSessionRetentionInformation() ([]*IE, error) {
	if i.Type != PFCPSessionRetentionInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
