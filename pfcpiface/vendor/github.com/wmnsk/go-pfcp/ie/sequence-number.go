// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSequenceNumber creates a new SequenceNumber IE.
func NewSequenceNumber(seq uint32) *IE {
	return newUint32ValIE(SequenceNumber, seq)
}

// SequenceNumber returns SequenceNumber in uint32 if the type of IE matches.
func (i *IE) SequenceNumber() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case SequenceNumber:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case LoadControlInformation:
		ies, err := i.LoadControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SequenceNumber {
				return x.SequenceNumber()
			}
		}
		return 0, ErrIENotFound
	case OverloadControlInformation:
		ies, err := i.OverloadControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SequenceNumber {
				return x.SequenceNumber()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}
