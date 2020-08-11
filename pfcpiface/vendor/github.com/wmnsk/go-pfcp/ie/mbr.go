// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewMBR creates a new MBR IE.
func NewMBR(ul, dl uint32) *IE {
	i := New(MBR, make([]byte, 8))

	binary.BigEndian.PutUint32(i.Payload[0:4], ul)
	binary.BigEndian.PutUint32(i.Payload[4:8], dl)

	return i
}

// MBR returns MBR in []byte if the type of IE matches.
func (i *IE) MBR() ([]byte, error) {
	if len(i.Payload) < 8 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MBR:
		return i.Payload, nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == MBR {
				return x.MBR()
			}
		}
		return nil, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == MBR {
				return x.MBR()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// MBRUL returns MBRUL in uint32 if the type of IE matches.
func (i *IE) MBRUL() (uint32, error) {
	v, err := i.MBR()
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(v[0:4]), nil
}

// MBRDL returns MBRDL in uint32 if the type of IE matches.
func (i *IE) MBRDL() (uint32, error) {
	v, err := i.MBR()
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(v[4:8]), nil
}
