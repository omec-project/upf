// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewErrorIndicationReport creates a new ErrorIndicationReport IE.
func NewErrorIndicationReport(fteid *IE) *IE {
	return newGroupedIE(ErrorIndicationReport, 0, fteid)
}

// ErrorIndicationReport returns the IEs above ErrorIndicationReport if the type of IE matches.
func (i *IE) ErrorIndicationReport() ([]*IE, error) {
	if i.Type != ErrorIndicationReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
