// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPFDContext creates a new PFDContext IE.
func NewPFDContext(contents ...*IE) *IE {
	return newGroupedIE(PFDContext, 0, contents...)
}

// PFDContext returns the IEs above PFDContext if the type of IE matches.
func (i *IE) PFDContext() ([]*IE, error) {
	switch i.Type {
	case PFDContext:
		return ParseMultiIEs(i.Payload)
	case ApplicationIDsPFDs:
		ies, err := i.ApplicationIDsPFDs()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PFDContext {
				return x.PFDContext()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
