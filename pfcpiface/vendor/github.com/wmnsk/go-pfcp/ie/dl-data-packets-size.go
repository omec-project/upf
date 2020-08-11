// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
)

// NewDLDataPacketsSize creates a new DLDataPacketsSize IE.
func NewDLDataPacketsSize(size uint16) *IE {
	return newUint16ValIE(DLDataPacketsSize, size)
}

// DLDataPacketsSize returns DLDataPacketsSize in uint16 if the type of IE matches.
func (i *IE) DLDataPacketsSize() (uint16, error) {
	if len(i.Payload) < 2 {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	switch i.Type {
	case DLDataPacketsSize:
		return binary.BigEndian.Uint16(i.Payload[0:2]), nil
	case DownlinkDataReport:
		ies, err := i.DownlinkDataReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DLDataPacketsSize {
				return x.DLDataPacketsSize()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
