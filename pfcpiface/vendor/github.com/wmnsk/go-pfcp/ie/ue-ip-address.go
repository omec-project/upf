// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewUEIPAddress creates a new UEIPAddress IE.
func NewUEIPAddress(flags uint8, v4, v6 string, v6d uint8) *IE {
	fields := NewUEIPAddressFields(flags, v4, v6, v6d)
	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(UEIPAddress, b)
}

// UEIPAddress returns UEIPAddress in *UEIPAddressFields if the type of IE matches.
func (i *IE) UEIPAddress() (*UEIPAddressFields, error) {
	switch i.Type {
	case UEIPAddress:
		fields, err := ParseUEIPAddressFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case CreatedPDR:
		ies, err := i.CreatedPDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case CreatedTrafficEndpoint:
		ies, err := i.CreatedTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UEIPAddress {
				return x.UEIPAddress()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasCH reports whether an IE has CH bit.
func (i *IE) HasCH() bool {
	if i.Type != UEIPAddress {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has5thBit(i.Payload[0])
}

// HasIPv6D reports whether an IE has IPv6D bit.
func (i *IE) HasIPv6D() bool {
	if i.Type != UEIPAddress {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has4thBit(i.Payload[0])
}

// HasSD reports whether an IE has SD bit.
func (i *IE) HasSD() bool {
	if i.Type != UEIPAddress {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has3rdBit(i.Payload[0])
}

// UEIPAddressFields represents a fields contained in UEIPAddress IE.
type UEIPAddressFields struct {
	Flags       uint8
	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  uint8
}

// NewUEIPAddressFields creates a new UEIPAddressFields.
func NewUEIPAddressFields(flags uint8, v4, v6 string, v6d uint8) *UEIPAddressFields {
	f := &UEIPAddressFields{Flags: flags}

	if has2ndBit(flags) && !has5thBit(flags) {
		f.IPv4Address = net.ParseIP(v4).To4()
	}

	if has1stBit(flags) && !has5thBit(flags) {
		f.IPv6Address = net.ParseIP(v6).To16()
	}

	if has4thBit(flags) {
		f.IPv6Prefix = v6d
	}

	return f
}

// ParseUEIPAddressFields parses b into UEIPAddressFields.
func ParseUEIPAddressFields(b []byte) (*UEIPAddressFields, error) {
	f := &UEIPAddressFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *UEIPAddressFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if has2ndBit(f.Flags) && !has5thBit(f.Flags) {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.IPv4Address = net.IP(b[offset : offset+4]).To4()
		offset += 4
	}

	if has1stBit(f.Flags) && !has5thBit(f.Flags) {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Address = net.IP(b[offset : offset+16]).To16()
		offset += 16
	}

	if has4thBit(f.Flags) {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Prefix = b[offset]
	}

	return nil
}

// Marshal returns the serialized bytes of UEIPAddressFields.
func (f *UEIPAddressFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *UEIPAddressFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if has2ndBit(f.Flags) && !has5thBit(f.Flags) {
		copy(b[offset:offset+4], f.IPv4Address)
		offset += 4
	}

	if has1stBit(f.Flags) && !has5thBit(f.Flags) {
		copy(b[offset:offset+16], f.IPv6Address)
		offset += 16
	}

	if has4thBit(f.Flags) {
		b[offset] = f.IPv6Prefix
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *UEIPAddressFields) MarshalLen() int {
	l := 1
	if has2ndBit(f.Flags) && !has5thBit(f.Flags) {
		l += 4
	}
	if has1stBit(f.Flags) && !has5thBit(f.Flags) {
		l += 16
	}
	if has4thBit(f.Flags) {
		l++
	}

	return l
}
