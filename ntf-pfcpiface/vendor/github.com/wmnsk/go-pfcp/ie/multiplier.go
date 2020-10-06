// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewMultiplier creates a new Multiplier IE.
func NewMultiplier(val uint64, exp uint32) *IE {
	i := New(Multiplier, make([]byte, 12))
	binary.BigEndian.PutUint64(i.Payload[0:8], val)
	binary.BigEndian.PutUint32(i.Payload[8:12], exp)

	return i
}

// Multiplier returns Multiplier in []byte if the type of IE matches.
func (i *IE) Multiplier() ([]byte, error) {
	switch i.Type {
	case Multiplier:
		return i.Payload, nil
	case AggregatedURRs:
		ies, err := i.AggregatedURRs()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == Multiplier {
				return x.Multiplier()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// ValueDigits returns ValueDigits in uint64 if the type of IE matches.
func (i *IE) ValueDigits() (uint64, error) {
	if len(i.Payload) < 8 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case Multiplier:
		return binary.BigEndian.Uint64(i.Payload[0:8]), nil
	case AggregatedURRs:
		ies, err := i.AggregatedURRs()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Multiplier {
				return x.ValueDigits()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// Exponent returns Exponent in uint32 if the type of IE matches.
func (i *IE) Exponent() (uint32, error) {
	if len(i.Payload) < 12 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case Multiplier:
		return binary.BigEndian.Uint32(i.Payload[8:12]), nil
	case AggregatedURRs:
		ies, err := i.AggregatedURRs()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Multiplier {
				return x.Exponent()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
