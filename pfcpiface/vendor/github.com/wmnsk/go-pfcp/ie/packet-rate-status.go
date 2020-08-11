// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewPacketRateStatus creates a new PacketRateStatus IE.
func NewPacketRateStatus(flags uint8, ul, aul, dl, adl uint16, vtime time.Time) *IE {
	f := NewPacketRateStatusFields(flags, ul, aul, dl, adl, vtime)

	b, err := f.Marshal()
	if err != nil {
		return nil
	}

	return New(PacketRateStatus, b)
}

// PacketRateStatus returns PacketRateStatus in structured format if the type of IE matches.
func (i *IE) PacketRateStatus() (*PacketRateStatusFields, error) {
	switch i.Type {
	case PacketRateStatus:
		f, err := ParsePacketRateStatusFields(i.Payload)
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
			if x.Type == PacketRateStatus {
				return x.PacketRateStatus()
			}
		}
		return nil, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketRateStatus {
				return x.PacketRateStatus()
			}
		}
		return nil, ErrIENotFound
	case PacketRateStatusReport:
		ies, err := i.PacketRateStatusReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketRateStatus {
				return x.PacketRateStatus()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// PacketRateStatusFields represents a f contained in PacketRateStatus IE.
type PacketRateStatusFields struct {
	Flags                                             uint8
	NumberOfRemainingUplinkPacketsAllowed             uint16
	NumberOfRemainingAdditionalUplinkPacketsAllowed   uint16
	NumberOfRemainingDownlinkPacketsAllowed           uint16
	NumberOfRemainingAdditionalDownlinkPacketsAllowed uint16
	RateControlStatusValidityTime                     time.Time
}

// NewPacketRateStatusFields creates a new NewPacketRateStatusFields.
func NewPacketRateStatusFields(flags uint8, ul, aul, dl, adl uint16, vtime time.Time) *PacketRateStatusFields {
	f := &PacketRateStatusFields{
		Flags:                         flags,
		RateControlStatusValidityTime: vtime,
	}

	if f.HasUL() {
		f.NumberOfRemainingUplinkPacketsAllowed = ul
		if f.HasAPR() {
			f.NumberOfRemainingAdditionalUplinkPacketsAllowed = aul
		}
	}

	if f.HasDL() {
		f.NumberOfRemainingDownlinkPacketsAllowed = dl
		if f.HasAPR() {
			f.NumberOfRemainingAdditionalDownlinkPacketsAllowed = adl
		}
	}

	return f
}

// HasAPR reports whether APR flag is set.
func (f *PacketRateStatusFields) HasAPR() bool {
	return has3rdBit(f.Flags)
}

// SetAPRFlag sets APR flag in PacketRateStatus.
func (f *PacketRateStatusFields) SetAPRFlag() {
	f.Flags |= 0x04
}

// HasDL reports whether DL flag is set.
func (f *PacketRateStatusFields) HasDL() bool {
	return has2ndBit(f.Flags)
}

// SetDLFlag sets DL flag in PacketRateStatus.
func (f *PacketRateStatusFields) SetDLFlag() {
	f.Flags |= 0x02
}

// HasUL reports whether UL flag is set.
func (f *PacketRateStatusFields) HasUL() bool {
	return has1stBit(f.Flags)
}

// SetULFlag sets UL flag in PacketRateStatus.
func (f *PacketRateStatusFields) SetULFlag() {
	f.Flags |= 0x01
}

// ParsePacketRateStatusFields parses b into PacketRateStatusFields.
func ParsePacketRateStatusFields(b []byte) (*PacketRateStatusFields, error) {
	f := &PacketRateStatusFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *PacketRateStatusFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasUL() {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		f.NumberOfRemainingUplinkPacketsAllowed = binary.BigEndian.Uint16(b[offset : offset+2])
		offset += 2

		if f.HasAPR() {
			if l < offset+2 {
				return io.ErrUnexpectedEOF
			}
			f.NumberOfRemainingAdditionalUplinkPacketsAllowed = binary.BigEndian.Uint16(b[offset : offset+2])
			offset += 2
		}
	}

	if f.HasDL() {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		f.NumberOfRemainingDownlinkPacketsAllowed = binary.BigEndian.Uint16(b[offset : offset+2])
		offset += 2

		if f.HasAPR() {
			if l < offset+2 {
				return io.ErrUnexpectedEOF
			}
			f.NumberOfRemainingAdditionalDownlinkPacketsAllowed = binary.BigEndian.Uint16(b[offset : offset+2])
			offset += 2
		}
	}

	if l < offset+8 {
		return io.ErrUnexpectedEOF
	}
	f.RateControlStatusValidityTime = time.Unix(int64(binary.BigEndian.Uint64(b[offset:offset+8])-2208988800), 0)

	return nil
}

// Marshal returns the serialized bytes of PacketRateStatusFields.
func (f *PacketRateStatusFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *PacketRateStatusFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasUL() {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint16(b[offset:offset+2], f.NumberOfRemainingUplinkPacketsAllowed)
		offset += 2

		if f.HasAPR() {
			if l < offset+2 {
				return io.ErrUnexpectedEOF
			}
			binary.BigEndian.PutUint16(b[offset:offset+2], f.NumberOfRemainingAdditionalUplinkPacketsAllowed)
			offset += 2
		}
	}

	if f.HasDL() {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint16(b[offset:offset+2], f.NumberOfRemainingDownlinkPacketsAllowed)
		offset += 2

		if f.HasAPR() {
			if l < offset+2 {
				return io.ErrUnexpectedEOF
			}
			binary.BigEndian.PutUint16(b[offset:offset+2], f.NumberOfRemainingAdditionalDownlinkPacketsAllowed)
			offset += 2
		}
	}

	if l < offset+8 {
		return io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint64(b[offset:offset+8], uint64(f.RateControlStatusValidityTime.Sub(time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)))/1000000000)

	return nil
}

// MarshalLen returns field length in integer.
func (f *PacketRateStatusFields) MarshalLen() int {
	l := 1 + 8
	if f.HasUL() {
		l += 2
		if f.HasAPR() {
			l += 2
		}
	}
	if f.HasDL() {
		l += 2
		if f.HasAPR() {
			l += 2
		}
	}

	return l
}
