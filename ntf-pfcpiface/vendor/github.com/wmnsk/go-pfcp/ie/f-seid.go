// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"net"
)

// NewFSEID creates a new FSEID IE.
func NewFSEID(teid uint64, v4, v6 net.IP, chid []byte) *IE {
	fields := NewFSEIDFields(teid, v4, v6, chid)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(FSEID, b)
}

// FSEID returns FSEID in structured format if the type of IE matches.
func (i *IE) FSEID() (*FSEIDFields, error) {
	if i.Type != FSEID {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	fields, err := ParseFSEIDFields(i.Payload)
	if err != nil {
		return nil, err
	}

	return fields, nil
}

// FSEIDFields represents a fields contained in FSEID IE.
type FSEIDFields struct {
	Flags       uint8
	SEID        uint64
	IPv4Address net.IP
	IPv6Address net.IP
	ChooseID    []byte
}

// NewFSEIDFields creates a new NewFSEIDFields.
func NewFSEIDFields(teid uint64, v4, v6 net.IP, chid []byte) *FSEIDFields {
	var fields *FSEIDFields
	if chid != nil {
		fields = &FSEIDFields{
			IPv4Address: v4,
			IPv6Address: v6,
			ChooseID:    chid,
		}
		fields.SetChIDFlag()

	} else {
		fields = &FSEIDFields{
			SEID:        teid,
			IPv4Address: v4,
			IPv6Address: v6,
		}
	}

	if v4 != nil {
		fields.SetIPv4Flag()
	}
	if v6 != nil {
		fields.SetIPv6Flag()
	}

	return fields
}

// HasChID reports whether CHID flag is set.
func (f *FSEIDFields) HasChID() bool {
	return has4thBit(f.Flags)
}

// SetChIDFlag sets CHID flag in FSEID.
func (f *FSEIDFields) SetChIDFlag() {
	f.Flags |= 0x08
}

// HasCh reports whether CH flag is set.
func (f *FSEIDFields) HasCh() bool {
	return has3rdBit(f.Flags)
}

// SetChFlag sets CH flag in FSEID.
func (f *FSEIDFields) SetChFlag() {
	f.Flags |= 0x04
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *FSEIDFields) HasIPv4() bool {
	return has2ndBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in FSEID.
func (f *FSEIDFields) SetIPv4Flag() {
	f.Flags |= 0x02
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *FSEIDFields) HasIPv6() bool {
	return has1stBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in FSEID.
func (f *FSEIDFields) SetIPv6Flag() {
	f.Flags |= 0x01
}

// ParseFSEIDFields parses b into FSEIDFields.
func ParseFSEIDFields(b []byte) (*FSEIDFields, error) {
	f := &FSEIDFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *FSEIDFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasChID() || f.HasCh() {
		if f.HasIPv4() {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			f.IPv4Address = net.IP(b[offset : offset+4])
			offset += 4
		}
		if f.HasIPv6() {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			f.IPv6Address = net.IP(b[offset : offset+16])
			offset += 16
		}

		if l <= offset {
			return nil
		}

		f.ChooseID = b[offset:]
		return nil
	}

	if l < offset+4 {
		return nil
	}
	f.SEID = binary.BigEndian.Uint64(b[offset : offset+8])
	offset += 8

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.IPv4Address = net.IP(b[offset : offset+4])
		offset += 4
	}
	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Address = net.IP(b[offset : offset+16])
	}

	return nil
}

// Marshal returns the serialized bytes of FSEIDFields.
func (f *FSEIDFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *FSEIDFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasChID() || f.HasCh() {
		if f.IPv4Address != nil {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+4], f.IPv4Address.To4())
			offset += 4
		}
		if f.IPv6Address != nil {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+16], f.IPv6Address.To16())
			offset += 16
		}

		copy(b[offset:], f.ChooseID)
		return nil
	}

	if l < offset+4 {
		return io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint64(b[offset:offset+8], f.SEID)
	offset += 8

	if f.IPv4Address != nil {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.IPv4Address.To4())
		offset += 4
	}
	if f.IPv6Address != nil {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.IPv6Address.To16())
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *FSEIDFields) MarshalLen() int {
	l := 1
	if f.IPv4Address != nil {
		l += 4
	}
	if f.IPv6Address != nil {
		l += 16
	}

	if f.HasChID() || f.HasCh() {
		l += len(f.ChooseID)
		return l
	}

	return l + 8
}
