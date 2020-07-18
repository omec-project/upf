// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// Interface definitions.
const (
	DstInterfaceAccess       uint8 = 0
	DstInterfaceCore         uint8 = 1
	DstInterfaceSGiLANN6LAN  uint8 = 2
	DstInterfaceCPFunction   uint8 = 3
	DstInterfaceLIFunction   uint8 = 4
	DstInterface5GVNInternal uint8 = 5
)

// NewDestinationInterface creates a new DestinationInterface IE.
func NewDestinationInterface(intf uint8) *IE {
	return newUint8ValIE(DestinationInterface, intf)
}

// DestinationInterface returns DestinationInterface in uint8 if the type of IE matches.
func (i *IE) DestinationInterface() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case DestinationInterface:
		return i.Payload[0], nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DestinationInterface {
				return x.DestinationInterface()
			}
		}
		return 0, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DestinationInterface {
				return x.DestinationInterface()
			}
		}
		return 0, ErrIENotFound
	case DuplicatingParameters:
		ies, err := i.DuplicatingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DestinationInterface {
				return x.DestinationInterface()
			}
		}
		return 0, ErrIENotFound
	case UpdateDuplicatingParameters:
		ies, err := i.UpdateDuplicatingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DestinationInterface {
				return x.DestinationInterface()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
