// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewNumberOfUEIPAddresses creates a new NumberOfUEIPAddresses IE.
func NewNumberOfUEIPAddresses(flags uint8, v4, v6 uint32) *IE {
	fields := NewNumberOfUEIPAddressesFields(flags, v4, v6)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(NumberOfUEIPAddresses, b)
}

// NumberOfUEIPAddresses returns NumberOfUEIPAddresses in structured format if the type of IE matches.
func (i *IE) NumberOfUEIPAddresses() (*NumberOfUEIPAddressesFields, error) {
	switch i.Type {
	case NumberOfUEIPAddresses:
		fields, err := ParseNumberOfUEIPAddressesFields(i.Payload)
		if err != nil {
			return nil, err
		}
		return fields, nil
	case UEIPAddressUsageInformation:
		ies, err := i.UEIPAddressUsageInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == NumberOfUEIPAddresses {
				return x.NumberOfUEIPAddresses()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}

}

// NumberOfUEIPAddressesFields represents a fields contained in NumberOfUEIPAddresses IE.
type NumberOfUEIPAddressesFields struct {
	Flags                   uint8
	NumberOfUEIPv4Addresses uint32
	NumberOfUEIPv6Addresses uint32
}

// NewNumberOfUEIPAddressesFields creates a new NewNumberOfUEIPAddressesFields.
func NewNumberOfUEIPAddressesFields(flags uint8, v4, v6 uint32) *NumberOfUEIPAddressesFields {
	return &NumberOfUEIPAddressesFields{
		Flags:                   flags,
		NumberOfUEIPv4Addresses: v4,
		NumberOfUEIPv6Addresses: v6,
	}
}

// HasNumIPv6 reports whether IPv6 flag is set.
func (f *NumberOfUEIPAddressesFields) HasNumIPv6() bool {
	return has2ndBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in NumberOfUEIPAddresses.
func (f *NumberOfUEIPAddressesFields) SetIPv6Flag() {
	f.Flags |= 0x02
}

// HasNumIPv4 reports whether IPv4 flag is set.
func (f *NumberOfUEIPAddressesFields) HasNumIPv4() bool {
	return has1stBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in NumberOfUEIPAddresses.
func (f *NumberOfUEIPAddressesFields) SetIPv4Flag() {
	f.Flags |= 0x01
}

// ParseNumberOfUEIPAddressesFields parses b into NumberOfUEIPAddressesFields.
func ParseNumberOfUEIPAddressesFields(b []byte) (*NumberOfUEIPAddressesFields, error) {
	f := &NumberOfUEIPAddressesFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *NumberOfUEIPAddressesFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasNumIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.NumberOfUEIPv4Addresses = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}

	if f.HasNumIPv6() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.NumberOfUEIPv6Addresses = binary.BigEndian.Uint32(b[offset : offset+4])
	}

	return nil
}

// Marshal returns the serialized bytes of NumberOfUEIPAddressesFields.
func (f *NumberOfUEIPAddressesFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *NumberOfUEIPAddressesFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasNumIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.NumberOfUEIPv4Addresses)
		offset += 4
	}

	if f.HasNumIPv6() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.NumberOfUEIPv6Addresses)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *NumberOfUEIPAddressesFields) MarshalLen() int {
	l := 1

	if f.HasNumIPv4() {
		l += 4
	}

	if f.HasNumIPv6() {
		l += 4
	}

	return l
}
