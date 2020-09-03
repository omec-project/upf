// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewSessionReport creates a new SessionReport IE.
func NewSessionReport(ies ...*IE) *IE {
	return newGroupedIE(SessionReport, 0, ies...)
}

// SessionReport returns the IEs above SessionReport if the type of IE matches.
func (i *IE) SessionReport() ([]*IE, error) {
	switch i.Type {
	case SessionReport:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
