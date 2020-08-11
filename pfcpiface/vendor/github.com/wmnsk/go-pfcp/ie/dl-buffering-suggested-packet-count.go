// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"math"
)

// NewDLBufferingSuggestedPacketCount creates a new DLBufferingSuggestedPacketCount IE.
func NewDLBufferingSuggestedPacketCount(count uint16) *IE {
	if count <= math.MaxUint8 {
		return newUint8ValIE(DLBufferingSuggestedPacketCount, uint8(count))
	}
	return newUint16ValIE(DLBufferingSuggestedPacketCount, count)
}

// DLBufferingSuggestedPacketCount returns DLBufferingSuggestedPacketCount in uint16 if the type of IE matches.
func (i *IE) DLBufferingSuggestedPacketCount() (uint16, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case DLBufferingSuggestedPacketCount:
		if i.Length == 1 {
			return uint16(i.Payload[0]), nil
		}

		if i.Length >= 2 && len(i.Payload) >= 2 {
			return binary.BigEndian.Uint16(i.Payload[0:2]), nil
		}

		return 0, io.ErrUnexpectedEOF
	case UpdateBARWithinSessionReportResponse:
		ies, err := i.CreateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DLBufferingSuggestedPacketCount {
				return x.DLBufferingSuggestedPacketCount()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
