// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFCPAssociationReleaseRequest creates a new PFCPAssociationReleaseRequest IE.
func NewPFCPAssociationReleaseRequest(sarr, urss int) *IE {
	return newUint8ValIE(PFCPAssociationReleaseRequest, uint8((urss<<1)|(sarr)))
}

// PFCPAssociationReleaseRequest returns PFCPAssociationReleaseRequest in uint8 if the type of IE matches.
func (i *IE) PFCPAssociationReleaseRequest() (uint8, error) {
	if i.Type != PFCPAssociationReleaseRequest {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload[0], nil
}

// HasURSS reports whether an IE has URSS bit.
func (i *IE) HasURSS() bool {
	v, err := i.PFCPAssociationReleaseRequest()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasSARR reports whether an IE has SARR bit.
func (i *IE) HasSARR() bool {
	v, err := i.PFCPAssociationReleaseRequest()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
