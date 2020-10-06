// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// Time Unit definitions
const (
	TimeUnitMinute   uint8 = 0
	TimeUnit6Minutes uint8 = 1
	TimeUnitHour     uint8 = 2
	TimeUnitDay      uint8 = 3
	TimeUnitWeek     uint8 = 4
)

// NewPacketRate creates a new PacketRate IE.
func NewPacketRate(flags uint8, ulunit uint8, ulpackets uint16, dlunit uint8, dlpackets uint16) *IE {
	fields := NewPacketRateFields(flags, ulunit, ulpackets, dlunit, dlpackets)
	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(PacketRate, b)
}

// PacketRate returns PacketRate in *PacketRateFields if the type of IE matches.
func (i *IE) PacketRate() (*PacketRateFields, error) {
	switch i.Type {
	case PacketRate:
		f, err := ParsePacketRateFields(i.Payload)
		if err != nil {
			return nil, err
		}
		return f, nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketRate {
				return x.PacketRate()
			}
		}
		return nil, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketRate {
				return x.PacketRate()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}

}

// HasDLPR reports whether an IE has DLPR bit.
func (i *IE) HasDLPR() bool {
	if i.Type != PacketRate {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has2ndBit(i.Payload[0])
}

// HasULPR reports whether an IE has ULPR bit.
func (i *IE) HasULPR() bool {
	if i.Type != PacketRate {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}

// PacketRateFields represents a fields contained in PacketRate IE.
type PacketRateFields struct {
	Flags              uint8
	UplinkTimeUnit     uint8
	DownlinkTimeUnit   uint8
	UplinkPacketRate   uint16
	DownlinkPacketRate uint16
}

// NewPacketRateFields creates a new PacketRateFields.
func NewPacketRateFields(flags uint8, ulunit uint8, ulpackets uint16, dlunit uint8, dlpackets uint16) *PacketRateFields {
	f := &PacketRateFields{Flags: flags}

	if has1stBit(flags) {
		f.UplinkTimeUnit = ulunit
		f.UplinkPacketRate = ulpackets
	}

	if has2ndBit(flags) {
		f.DownlinkTimeUnit = dlunit
		f.DownlinkPacketRate = dlpackets
	}

	return f
}

// ParsePacketRateFields parses b into PacketRateFields.
func ParsePacketRateFields(b []byte) (*PacketRateFields, error) {
	f := &PacketRateFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *PacketRateFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if has1stBit(f.Flags) {
		if l < offset+3 {
			return io.ErrUnexpectedEOF
		}
		f.UplinkTimeUnit = b[offset]
		f.UplinkPacketRate = binary.BigEndian.Uint16(b[offset+1 : offset+3])
		offset += 3
	}

	if has2ndBit(f.Flags) {
		if l < offset+3 {
			return io.ErrUnexpectedEOF
		}
		f.DownlinkTimeUnit = b[offset]
		f.DownlinkPacketRate = binary.BigEndian.Uint16(b[offset+1 : offset+3])
	}

	return nil
}

// Marshal returns the serialized bytes of PacketRateFields.
func (f *PacketRateFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *PacketRateFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if has1stBit(f.Flags) {
		b[offset] = f.UplinkTimeUnit
		binary.BigEndian.PutUint16(b[offset+1:offset+3], f.UplinkPacketRate)
		offset += 3
	}

	if has2ndBit(f.Flags) {
		b[offset] = f.DownlinkTimeUnit
		binary.BigEndian.PutUint16(b[offset+1:offset+3], f.DownlinkPacketRate)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *PacketRateFields) MarshalLen() int {
	l := 1
	if has1stBit(f.Flags) {
		l += 3
	}
	if has2ndBit(f.Flags) {
		l += 3
	}

	return l
}
