// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewAggregatedURRID creates a new AggregatedURRID IE.
func NewAggregatedURRID(id uint32) *IE {
	return newUint32ValIE(AggregatedURRID, id)
}

// AggregatedURRID returns AggregatedURRID in uint32 if the type of IE matches.
func (i *IE) AggregatedURRID() (uint32, error) {
	switch i.Type {
	case AggregatedURRID:
		if len(i.Payload) < 4 {
			return 0, io.ErrUnexpectedEOF
		}

		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case AggregatedURRs:
		ies, err := i.AggregatedURRs()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == AggregatedURRID {
				return x.AggregatedURRID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
