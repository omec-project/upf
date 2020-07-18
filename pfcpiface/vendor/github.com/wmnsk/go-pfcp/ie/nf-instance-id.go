// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewNFInstanceID creates a new NFInstanceID IE.
func NewNFInstanceID(id []byte) *IE {
	return New(NFInstanceID, id[:16])
}

// NFInstanceID returns NFInstanceID in []byte if the type of IE matches.
func (i *IE) NFInstanceID() ([]byte, error) {
	if len(i.Payload) < 16 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case NFInstanceID:
		return i.Payload[:16], nil
	case EthernetPacketFilter:
		ies, err := i.EthernetPacketFilter()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == NFInstanceID {
				return x.NFInstanceID()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
