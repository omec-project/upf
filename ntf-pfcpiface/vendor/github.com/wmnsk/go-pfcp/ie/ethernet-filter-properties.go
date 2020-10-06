// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewEthernetFilterProperties creates a new EthernetFilterProperties IE.
func NewEthernetFilterProperties(props uint8) *IE {
	return newUint8ValIE(EthernetFilterProperties, props)
}

// EthernetFilterProperties returns EthernetFilterProperties in []byte if the type of IE matches.
func (i *IE) EthernetFilterProperties() ([]byte, error) {
	if len(i.Payload) < 1 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case EthernetFilterProperties:
		return i.Payload, nil
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetPacketFilter {
				return x.EthernetFilterProperties()
			}
		}
		return nil, ErrIENotFound
	case EthernetPacketFilter:
		ies, err := i.EthernetPacketFilter()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetFilterProperties {
				return x.EthernetFilterProperties()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasBIDE reports whether an IE has BIDE bit.
func (i *IE) HasBIDE() bool {
	v, err := i.EthernetFilterProperties()
	if err != nil {
		return false
	}

	return has1stBit(v[0])
}
