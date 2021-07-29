// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"net"
)

// NewFSEID creates a new FSEID IE.
func NewFSEID(seid uint64, v4, v6 net.IP) *IE {
	fields := NewFSEIDFields(seid, v4, v6)

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
}

// NewFSEIDFields creates a new NewFSEIDFields.
func NewFSEIDFields(seid uint64, v4, v6 net.IP) *FSEIDFields {
	f := &FSEIDFields{
		SEID:        seid,
		IPv4Address: v4,
		IPv6Address: v6,
	}

	if v4 != nil {
		f.SetIPv4Flag()
	}

	if v6 != nil {
		f.SetIPv6Flag()
	}

	return f
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
	if l < 9 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

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
	if l < 9 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	binary.BigEndian.PutUint64(b[offset:offset+8], f.SEID)
	offset += 8

	if f.HasIPv4() && f.IPv4Address != nil {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.IPv4Address.To4())
		offset += 4
	}

	if f.HasIPv6() && f.IPv6Address != nil {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.IPv6Address.To16())
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *FSEIDFields) MarshalLen() int {
	l := 9

	if f.IPv4Address != nil {
		l += 4
	}

	if f.IPv6Address != nil {
		l += 16
	}

	return l
}
