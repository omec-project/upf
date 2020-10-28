// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewEthernetFilterProperties creates a new EthernetFilterProperties IE.
func NewEthernetFilterProperties(props uint8) *IE {
	return newUint8ValIE(EthernetFilterProperties, props)
}

// EthernetFilterProperties returns EthernetFilterProperties in uint8 if the type of IE matches.
func (i *IE) EthernetFilterProperties() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case EthernetFilterProperties:
		return i.Payload[0], nil
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetPacketFilter {
				return x.EthernetFilterProperties()
			}
		}
		return 0, ErrIENotFound
	case EthernetPacketFilter:
		ies, err := i.EthernetPacketFilter()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetFilterProperties {
				return x.EthernetFilterProperties()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasBIDE reports whether an IE has BIDE bit.
func (i *IE) HasBIDE() bool {
	v, err := i.EthernetFilterProperties()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
