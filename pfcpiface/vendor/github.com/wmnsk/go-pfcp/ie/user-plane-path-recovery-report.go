// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUserPlanePathRecoveryReport creates a new UserPlanePathRecoveryReport IE.
func NewUserPlanePathRecoveryReport(peer *IE) *IE {
	return newGroupedIE(UserPlanePathRecoveryReport, 0, peer)
}

// UserPlanePathRecoveryReport returns the IEs above UserPlanePathRecoveryReport if the type of IE matches.
func (i *IE) UserPlanePathRecoveryReport() ([]*IE, error) {
	if i.Type != UserPlanePathRecoveryReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
