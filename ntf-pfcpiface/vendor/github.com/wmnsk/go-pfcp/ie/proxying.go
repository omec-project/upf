// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewProxying creates a new Proxying IE.
func NewProxying(ins, arp uint8) *IE {
	return newUint8ValIE(Proxying, (ins<<1)|arp)
}

// Proxying returns Proxying in uint8 if the type of IE matches.
func (i *IE) Proxying() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case Proxying:
		return i.Payload[0], nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Proxying {
				return x.Proxying()
			}
		}
		return 0, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Proxying {
				return x.Proxying()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasINS reports whether an IE has INS bit.
func (i *IE) HasINS() bool {
	v, err := i.Proxying()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasARP reports whether an IE has ARP bit.
func (i *IE) HasARP() bool {
	v, err := i.Proxying()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
