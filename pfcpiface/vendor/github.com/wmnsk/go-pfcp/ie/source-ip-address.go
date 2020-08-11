// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewSourceIPAddress creates a new SourceIPAddress IE.
func NewSourceIPAddress(v4, v6 net.IP, mpl uint8) *IE {
	fields := NewSourceIPAddressFields(v4, v6, mpl)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(SourceIPAddress, b)
}

// SourceIPAddress returns SourceIPAddress in structured format if the type of IE matches.
func (i *IE) SourceIPAddress() (*SourceIPAddressFields, error) {
	switch i.Type {
	case SourceIPAddress:
		fields, err := ParseSourceIPAddressFields(i.Payload)
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
			if x.Type == IPMulticastAddressingInfo {
				return x.SourceIPAddress()
			}
		}
		return nil, ErrIENotFound
	case IPMulticastAddressingInfo:
		ies, err := i.IPMulticastAddressingInfo()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SourceIPAddress {
				return x.SourceIPAddress()
			}
		}
		return nil, ErrIENotFound
	case JoinIPMulticastInformationWithinUsageReport:
		ies, err := i.JoinIPMulticastInformationWithinUsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SourceIPAddress {
				return x.SourceIPAddress()
			}
		}
		return nil, ErrIENotFound
	case LeaveIPMulticastInformationWithinUsageReport:
		ies, err := i.LeaveIPMulticastInformationWithinUsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SourceIPAddress {
				return x.SourceIPAddress()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// SourceIPAddressFields represents a fields contained in SourceIPAddress IE.
type SourceIPAddressFields struct {
	Flags            uint8
	IPv4Address      net.IP
	IPv6Address      net.IP
	MaskPrefixLength uint8
}

// NewSourceIPAddressFields creates a new NewSourceIPAddressFields.
func NewSourceIPAddressFields(v4, v6 net.IP, mpl uint8) *SourceIPAddressFields {
	f := &SourceIPAddressFields{Flags: 0x00}

	if v4 != nil {
		f.SetIPv4Flag()
		f.IPv4Address = v4
	}
	if v6 != nil {
		f.SetIPv6Flag()
		f.IPv6Address = v6
	}

	if mpl != 0 {
		f.SetMPLFlag()
		f.MaskPrefixLength = mpl
	}

	return f
}

// HasMPL reports whether MPL flag is set.
func (f *SourceIPAddressFields) HasMPL() bool {
	return has3rdBit(f.Flags)
}

// SetMPLFlag sets MPL flag in SourceIPAddress.
func (f *SourceIPAddressFields) SetMPLFlag() {
	f.Flags |= 0x04
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *SourceIPAddressFields) HasIPv4() bool {
	return has2ndBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in SourceIPAddress.
func (f *SourceIPAddressFields) SetIPv4Flag() {
	f.Flags |= 0x02
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *SourceIPAddressFields) HasIPv6() bool {
	return has1stBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in SourceIPAddress.
func (f *SourceIPAddressFields) SetIPv6Flag() {
	f.Flags |= 0x01
}

// ParseSourceIPAddressFields parses b into SourceIPAddressFields.
func ParseSourceIPAddressFields(b []byte) (*SourceIPAddressFields, error) {
	f := &SourceIPAddressFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *SourceIPAddressFields) UnmarshalBinary(b []byte) error {
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
		offset += 16
	}

	if f.HasMPL() {
		if l < offset {
			return io.ErrUnexpectedEOF
		}
		f.MaskPrefixLength = b[offset]
	}

	return nil
}

// Marshal returns the serialized bytes of SourceIPAddressFields.
func (f *SourceIPAddressFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *SourceIPAddressFields) MarshalTo(b []byte) error {
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
		copy(b[offset:offset+4], f.IPv4Address.To4())
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.IPv6Address.To16())
		offset += 16
	}

	if f.HasMPL() {
		if l < offset {
			return io.ErrUnexpectedEOF
		}
		b[offset] = f.MaskPrefixLength
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *SourceIPAddressFields) MarshalLen() int {
	l := 1
	if f.HasIPv4() {
		l += 4
	}
	if f.HasIPv6() {
		l += 16
	}
	if f.HasMPL() {
		l += 1
	}

	return l
}
