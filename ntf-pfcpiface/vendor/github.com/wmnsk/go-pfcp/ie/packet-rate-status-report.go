// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewPacketRateStatusReport creates a new PacketRateStatusReport IE.
func NewPacketRateStatusReport(ies ...*IE) *IE {
	return newGroupedIE(PacketRateStatusReport, 0, ies...)
}

// PacketRateStatusReport returns the IEs above PacketRateStatusReport if the type of IE matches.
func (i *IE) PacketRateStatusReport() ([]*IE, error) {
	switch i.Type {
	case PacketRateStatusReport:
		return ParseMultiIEs(i.Payload)
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
