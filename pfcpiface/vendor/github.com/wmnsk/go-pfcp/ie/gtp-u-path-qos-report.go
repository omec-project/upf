// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewGTPUPathQoSReport creates a new GTPUPathQoSReport IE.
func NewGTPUPathQoSReport(ies ...*IE) *IE {
	return newGroupedIE(GTPUPathQoSReport, 0, ies...)
}

// GTPUPathQoSReport returns the IEs above GTPUPathQoSReport if the type of IE matches.
func (i *IE) GTPUPathQoSReport() ([]*IE, error) {
	if i.Type != GTPUPathQoSReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
