// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewEthernetFilterID creates a new EthernetFilterID IE.
func NewEthernetFilterID(id uint32) *IE {
	return newUint32ValIE(EthernetFilterID, id)
}

// EthernetFilterID returns EthernetFilterID in uint32 if the type of IE matches.
func (i *IE) EthernetFilterID() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case EthernetFilterID:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetPacketFilter {
				return x.EthernetFilterID()
			}
		}
		return 0, ErrIENotFound
	case EthernetPacketFilter:
		ies, err := i.EthernetPacketFilter()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetFilterID {
				return x.EthernetFilterID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}
