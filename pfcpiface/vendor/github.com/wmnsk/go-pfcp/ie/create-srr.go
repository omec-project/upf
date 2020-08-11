// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateSRR creates a new CreateSRR IE.
func NewCreateSRR(srr, availability, qos *IE) *IE {
	return newGroupedIE(CreateSRR, 0, srr, availability, qos)
}

// CreateSRR returns the IEs above CreateSRR if the type of IE matches.
func (i *IE) CreateSRR() ([]*IE, error) {
	switch i.Type {
	case CreateSRR:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
