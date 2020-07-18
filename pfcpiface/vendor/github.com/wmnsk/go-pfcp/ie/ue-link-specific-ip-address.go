// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewUELinkSpecificIPAddress creates a new UELinkSpecificIPAddress IE.
func NewUELinkSpecificIPAddress(v4, v6, nv4, nv6 net.IP) *IE {
	fields := NewUELinkSpecificIPAddressFields(v4, v6, nv4, nv6)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(UELinkSpecificIPAddress, b)
}

// UELinkSpecificIPAddress returns UELinkSpecificIPAddress in structured format if the type of IE matches.
func (i *IE) UELinkSpecificIPAddress() (*UELinkSpecificIPAddressFields, error) {
	switch i.Type {
	case UELinkSpecificIPAddress:
		fields, err := ParseUELinkSpecificIPAddressFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case ATSSSControlParameters:
		ies, err := i.ATSSSControlParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == MPTCPParameters {
				return x.UELinkSpecificIPAddress()
			}
		}
		return nil, ErrIENotFound
	case MPTCPParameters:
		ies, err := i.MPTCPParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UELinkSpecificIPAddress {
				return x.UELinkSpecificIPAddress()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// UELinkSpecificIPAddressFields represents a fields contained in UELinkSpecificIPAddress IE.
type UELinkSpecificIPAddressFields struct {
	Flags                                     uint8
	UELinkSpecificIPv4AddressFor3GPPAccess    net.IP
	UELinkSpecificIPv6AddressFor3GPPAccess    net.IP
	UELinkSpecificIPv4AddressForNon3GPPAccess net.IP
	UELinkSpecificIPv6AddressForNon3GPPAccess net.IP
}

// NewUELinkSpecificIPAddressFields creates a new NewUELinkSpecificIPAddressFields.
func NewUELinkSpecificIPAddressFields(v4, v6, nv4, nv6 net.IP) *UELinkSpecificIPAddressFields {
	f := &UELinkSpecificIPAddressFields{Flags: 0x00}

	if v4 != nil {
		f.SetIPv4Flag()
		f.UELinkSpecificIPv4AddressFor3GPPAccess = v4
	}
	if v6 != nil {
		f.SetIPv6Flag()
		f.UELinkSpecificIPv6AddressFor3GPPAccess = v6
	}

	if nv4 != nil {
		f.SetNV4Flag()
		f.UELinkSpecificIPv4AddressForNon3GPPAccess = nv4
	}
	if nv6 != nil {
		f.SetNV6Flag()
		f.UELinkSpecificIPv6AddressForNon3GPPAccess = nv6
	}

	return f
}

// HasNV6 reports whether NV6 flag is set.
func (f *UELinkSpecificIPAddressFields) HasNV6() bool {
	return has4thBit(f.Flags)
}

// SetNV6Flag sets NV6 flag in UELinkSpecificIPAddress.
func (f *UELinkSpecificIPAddressFields) SetNV6Flag() {
	f.Flags |= 0x08
}

// HasNV4 reports whether NV4 flag is set.
func (f *UELinkSpecificIPAddressFields) HasNV4() bool {
	return has3rdBit(f.Flags)
}

// SetNV4Flag sets NV4 flag in UELinkSpecificIPAddress.
func (f *UELinkSpecificIPAddressFields) SetNV4Flag() {
	f.Flags |= 0x04
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *UELinkSpecificIPAddressFields) HasIPv6() bool {
	return has2ndBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in UELinkSpecificIPAddress.
func (f *UELinkSpecificIPAddressFields) SetIPv6Flag() {
	f.Flags |= 0x02
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *UELinkSpecificIPAddressFields) HasIPv4() bool {
	return has1stBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in UELinkSpecificIPAddress.
func (f *UELinkSpecificIPAddressFields) SetIPv4Flag() {
	f.Flags |= 0x01
}

// ParseUELinkSpecificIPAddressFields parses b into UELinkSpecificIPAddressFields.
func ParseUELinkSpecificIPAddressFields(b []byte) (*UELinkSpecificIPAddressFields, error) {
	f := &UELinkSpecificIPAddressFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *UELinkSpecificIPAddressFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.UELinkSpecificIPv4AddressFor3GPPAccess = net.IP(b[offset : offset+4])
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.UELinkSpecificIPv6AddressFor3GPPAccess = net.IP(b[offset : offset+16])
		offset += 16
	}

	if f.HasNV4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.UELinkSpecificIPv4AddressForNon3GPPAccess = net.IP(b[offset : offset+4])
		offset += 4
	}

	if f.HasNV6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.UELinkSpecificIPv6AddressForNon3GPPAccess = net.IP(b[offset : offset+16])
	}

	return nil
}

// Marshal returns the serialized bytes of UELinkSpecificIPAddressFields.
func (f *UELinkSpecificIPAddressFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *UELinkSpecificIPAddressFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.UELinkSpecificIPv4AddressFor3GPPAccess.To4())
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.UELinkSpecificIPv6AddressFor3GPPAccess.To16())
		offset += 16
	}

	if f.HasNV4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.UELinkSpecificIPv4AddressForNon3GPPAccess.To4())
		offset += 4
	}

	if f.HasNV6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.UELinkSpecificIPv6AddressForNon3GPPAccess.To16())
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *UELinkSpecificIPAddressFields) MarshalLen() int {
	l := 1
	if f.HasIPv4() {
		l += 4
	}
	if f.HasIPv6() {
		l += 16
	}
	if f.HasNV4() {
		l += 4
	}
	if f.HasNV6() {
		l += 16
	}

	return l
}
