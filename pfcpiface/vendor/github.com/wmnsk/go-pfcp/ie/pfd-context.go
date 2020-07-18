// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFDContext creates a new PFDContext IE.
func NewPFDContext(contents *IE) *IE {
	return newGroupedIE(PFDContext, 0, contents)
}

// PFDContext returns the IEs above PFDContext if the type of IE matches.
func (i *IE) PFDContext() ([]*IE, error) {
	if i.Type != PFDContext {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
