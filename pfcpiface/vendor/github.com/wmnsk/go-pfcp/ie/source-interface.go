// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// Interface definitions.
const (
	SrcInterfaceAccess       uint8 = 0
	SrcInterfaceCore         uint8 = 1
	SrcInterfaceSGiLANN6LAN  uint8 = 2
	SrcInterfaceCPFunction   uint8 = 3
	SrcInterface5GVNInternal uint8 = 4
)

// NewSourceInterface creates a new SourceInterface IE.
func NewSourceInterface(intf uint8) *IE {
	return newUint8ValIE(SourceInterface, intf)
}

// SourceInterface returns SourceInterface in uint8 if the type of IE matches.
func (i *IE) SourceInterface() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case SourceInterface:
		return i.Payload[0], nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.SourceInterface()
			}
		}
		return 0, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.SourceInterface()
			}
		}
		return 0, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SourceInterface {
				return x.SourceInterface()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
