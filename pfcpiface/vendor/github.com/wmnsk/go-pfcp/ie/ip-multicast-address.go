// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewIPMulticastAddress creates a new IPMulticastAddress IE.
func NewIPMulticastAddress(sv4, sv6, ev4, ev6 net.IP) *IE {
	fields := NewIPMulticastAddressFields(sv4, sv6, ev4, ev6)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(IPMulticastAddress, b)
}

// IPMulticastAddress returns IPMulticastAddress in structured format if the type of IE matches.
func (i *IE) IPMulticastAddress() (*IPMulticastAddressFields, error) {
	switch i.Type {
	case IPMulticastAddress:
		fields, err := ParseIPMulticastAddressFields(i.Payload)
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
				return x.IPMulticastAddress()
			}
		}
		return nil, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddressingInfo {
				return x.IPMulticastAddress()
			}
		}
		return nil, ErrIENotFound
	case IPMulticastAddressingInfo:
		ies, err := i.IPMulticastAddressingInfo()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddress {
				return x.IPMulticastAddress()
			}
		}
		return nil, ErrIENotFound
	case JoinIPMulticastInformationWithinUsageReport:
		ies, err := i.JoinIPMulticastInformationWithinUsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddress {
				return x.IPMulticastAddress()
			}
		}
		return nil, ErrIENotFound
	case LeaveIPMulticastInformationWithinUsageReport:
		ies, err := i.LeaveIPMulticastInformationWithinUsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddress {
				return x.IPMulticastAddress()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// IPMulticastAddressFields represents a fields contained in IPMulticastAddress IE.
type IPMulticastAddressFields struct {
	Flags            uint8
	StartIPv4Address net.IP
	StartIPv6Address net.IP
	EndIPv4Address   net.IP
	EndIPv6Address   net.IP
}

// NewIPMulticastAddressFields creates a new NewIPMulticastAddressFields.
func NewIPMulticastAddressFields(sv4, sv6, ev4, ev6 net.IP) *IPMulticastAddressFields {
	f := &IPMulticastAddressFields{Flags: 0x00}
	if sv4 == nil && sv6 == nil {
		f.SetAnyFlag()
		return f
	}

	if sv4 != nil {
		f.SetIPv4Flag()
		f.StartIPv4Address = sv4
	}
	if sv6 != nil {
		f.SetIPv6Flag()
		f.StartIPv6Address = sv6
	}

	if ev4 != nil {
		f.SetRangeFlag()
		f.EndIPv4Address = ev4
	}

	if ev6 != nil {
		f.SetRangeFlag()
		f.EndIPv6Address = ev6
	}

	return f
}

// HasAny reports whether Any flag is set.
func (f *IPMulticastAddressFields) HasAny() bool {
	return has4thBit(f.Flags)
}

// SetAnyFlag sets Any flag in IPMulticastAddress.
func (f *IPMulticastAddressFields) SetAnyFlag() {
	f.Flags |= 0x08
}

// HasRange reports whether Range flag is set.
func (f *IPMulticastAddressFields) HasRange() bool {
	return has3rdBit(f.Flags)
}

// SetRangeFlag sets Range flag in IPMulticastAddress.
func (f *IPMulticastAddressFields) SetRangeFlag() {
	f.Flags |= 0x04
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *IPMulticastAddressFields) HasIPv4() bool {
	return has2ndBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in IPMulticastAddress.
func (f *IPMulticastAddressFields) SetIPv4Flag() {
	f.Flags |= 0x02
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *IPMulticastAddressFields) HasIPv6() bool {
	return has1stBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in IPMulticastAddress.
func (f *IPMulticastAddressFields) SetIPv6Flag() {
	f.Flags |= 0x01
}

// ParseIPMulticastAddressFields parses b into IPMulticastAddressFields.
func ParseIPMulticastAddressFields(b []byte) (*IPMulticastAddressFields, error) {
	f := &IPMulticastAddressFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *IPMulticastAddressFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasAny() {
		return nil
	}

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.StartIPv4Address = net.IP(b[offset : offset+4])
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.StartIPv6Address = net.IP(b[offset : offset+16])
		offset += 16
	}

	if f.HasRange() {

		if f.HasIPv4() {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			f.EndIPv4Address = net.IP(b[offset : offset+4])
			offset += 4
		}

		if f.HasIPv6() {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			f.EndIPv6Address = net.IP(b[offset : offset+16])
		}
	}

	return nil
}

// Marshal returns the serialized bytes of IPMulticastAddressFields.
func (f *IPMulticastAddressFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *IPMulticastAddressFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasAny() {
		return nil
	}

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.StartIPv4Address.To4())
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.StartIPv6Address.To16())
		offset += 16
	}

	if f.HasRange() {
		if f.HasIPv4() {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+4], f.EndIPv4Address.To4())
			offset += 4
		}

		if f.HasIPv6() {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+16], f.EndIPv6Address.To16())
		}
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *IPMulticastAddressFields) MarshalLen() int {
	l := 1
	if f.StartIPv4Address != nil {
		l += 4
	}
	if f.StartIPv6Address != nil {
		l += 16
	}
	if f.EndIPv4Address != nil {
		l += 4
	}
	if f.EndIPv6Address != nil {
		l += 16
	}

	return l
}
