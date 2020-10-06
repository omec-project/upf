// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewCPPFCPEntityIPAddress creates a new CPPFCPEntityIPAddress IE.
func NewCPPFCPEntityIPAddress(v4, v6 net.IP) *IE {
	fields := NewCPPFCPEntityIPAddressFields(v4, v6)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(CPPFCPEntityIPAddress, b)
}

// CPPFCPEntityIPAddress returns CPPFCPEntityIPAddress in structured format if the type of IE matches.
func (i *IE) CPPFCPEntityIPAddress() (*CPPFCPEntityIPAddressFields, error) {
	switch i.Type {
	case CPPFCPEntityIPAddress:
		fields, err := ParseCPPFCPEntityIPAddressFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case PFCPSessionRetentionInformation:
		ies, err := i.PFCPSessionRetentionInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == CPPFCPEntityIPAddress {
				return x.CPPFCPEntityIPAddress()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// CPPFCPEntityIPAddressFields represents a fields contained in CPPFCPEntityIPAddress IE.
type CPPFCPEntityIPAddressFields struct {
	Flags       uint8
	TEID        uint32
	IPv4Address net.IP
	IPv6Address net.IP
	ChooseID    []byte
}

// NewCPPFCPEntityIPAddressFields creates a new NewCPPFCPEntityIPAddressFields.
func NewCPPFCPEntityIPAddressFields(v4, v6 net.IP) *CPPFCPEntityIPAddressFields {
	f := &CPPFCPEntityIPAddressFields{}

	if v4 != nil {
		f.IPv4Address = v4
		f.SetIPv4Flag()
	}
	if v6 != nil {
		f.IPv6Address = v6
		f.SetIPv6Flag()
	}

	return f
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *CPPFCPEntityIPAddressFields) HasIPv4() bool {
	return has2ndBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in CPPFCPEntityIPAddress.
func (f *CPPFCPEntityIPAddressFields) SetIPv4Flag() {
	f.Flags |= 0x02
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *CPPFCPEntityIPAddressFields) HasIPv6() bool {
	return has1stBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in CPPFCPEntityIPAddress.
func (f *CPPFCPEntityIPAddressFields) SetIPv6Flag() {
	f.Flags |= 0x01
}

// ParseCPPFCPEntityIPAddressFields parses b into CPPFCPEntityIPAddressFields.
func ParseCPPFCPEntityIPAddressFields(b []byte) (*CPPFCPEntityIPAddressFields, error) {
	f := &CPPFCPEntityIPAddressFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *CPPFCPEntityIPAddressFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

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

// Marshal returns the serialized bytes of CPPFCPEntityIPAddressFields.
func (f *CPPFCPEntityIPAddressFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *CPPFCPEntityIPAddressFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

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
func (f *CPPFCPEntityIPAddressFields) MarshalLen() int {
	l := 1
	if f.IPv4Address != nil {
		l += 4
	}
	if f.IPv6Address != nil {
		l += 16
	}

	return l
}
