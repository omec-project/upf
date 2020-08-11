// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUserPlanePathFailureReport creates a new UserPlanePathFailureReport IE.
func NewUserPlanePathFailureReport(peer *IE) *IE {
	return newGroupedIE(UserPlanePathFailureReport, 0, peer)
}

// UserPlanePathFailureReport returns the IEs above UserPlanePathFailureReport if the type of IE matches.
func (i *IE) UserPlanePathFailureReport() ([]*IE, error) {
	if i.Type != UserPlanePathFailureReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
