// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewNWTTPortNumber creates a new NWTTPortNumber IE.
func NewNWTTPortNumber(port uint32) *IE {
	return newUint32ValIE(NWTTPortNumber, port)
}

// NWTTPortNumber returns NWTTPortNumber in uint32 if the type of IE matches.
func (i *IE) NWTTPortNumber() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case NWTTPortNumber:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreatedBridgeInfoForTSC:
		ies, err := i.CreatedBridgeInfoForTSC()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == NWTTPortNumber {
				return x.NWTTPortNumber()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}
