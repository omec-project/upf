// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"net"
)

// MPTCP Proxy Type definitions(TS24.193).
const (
	MPTCPProxyTransportConverter uint8 = 1
)

// NewMPTCPAddressInformation creates a new MPTCPAddressInformation IE.
func NewMPTCPAddressInformation(ptype uint8, port uint16, v4, v6 net.IP) *IE {
	fields := NewMPTCPAddressInformationFields(ptype, port, v4, v6)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(MPTCPAddressInformation, b)
}

// MPTCPAddressInformation returns MPTCPAddressInformation in structured format if the type of IE matches.
func (i *IE) MPTCPAddressInformation() (*MPTCPAddressInformationFields, error) {
	switch i.Type {
	case MPTCPAddressInformation:
		fields, err := ParseMPTCPAddressInformationFields(i.Payload)
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
				return x.MPTCPAddressInformation()
			}
		}
		return nil, ErrIENotFound
	case MPTCPParameters:
		ies, err := i.MPTCPParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == MPTCPAddressInformation {
				return x.MPTCPAddressInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// MPTCPAddressInformationFields represents a fields contained in MPTCPAddressInformation IE.
type MPTCPAddressInformationFields struct {
	Flags            uint8
	MPTCPProxyType   uint8
	MPTCPProxyPort   uint16
	MPTCPIPv4Address net.IP
	MPTCPIPv6Address net.IP
}

// NewMPTCPAddressInformationFields creates a new NewMPTCPAddressInformationFields.
func NewMPTCPAddressInformationFields(ptype uint8, port uint16, v4, v6 net.IP) *MPTCPAddressInformationFields {
	f := &MPTCPAddressInformationFields{
		Flags:          0x00,
		MPTCPProxyType: ptype,
		MPTCPProxyPort: port,
	}

	if v4 != nil {
		f.SetIPv4Flag()
		f.MPTCPIPv4Address = v4
	}
	if v6 != nil {
		f.SetIPv6Flag()
		f.MPTCPIPv6Address = v6
	}

	return f
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *MPTCPAddressInformationFields) HasIPv6() bool {
	return has2ndBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in MPTCPAddressInformation.
func (f *MPTCPAddressInformationFields) SetIPv6Flag() {
	f.Flags |= 0x02
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *MPTCPAddressInformationFields) HasIPv4() bool {
	return has1stBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in MPTCPAddressInformation.
func (f *MPTCPAddressInformationFields) SetIPv4Flag() {
	f.Flags |= 0x01
}

// ParseMPTCPAddressInformationFields parses b into MPTCPAddressInformationFields.
func ParseMPTCPAddressInformationFields(b []byte) (*MPTCPAddressInformationFields, error) {
	f := &MPTCPAddressInformationFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *MPTCPAddressInformationFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 4 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	f.MPTCPProxyType = b[1]
	f.MPTCPProxyPort = binary.BigEndian.Uint16(b[2:4])
	offset := 4

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.MPTCPIPv4Address = net.IP(b[offset : offset+4])
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.MPTCPIPv6Address = net.IP(b[offset : offset+16])
	}

	return nil
}

// Marshal returns the serialized bytes of MPTCPAddressInformationFields.
func (f *MPTCPAddressInformationFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *MPTCPAddressInformationFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 4 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	b[1] = f.MPTCPProxyType
	binary.BigEndian.PutUint16(b[2:4], f.MPTCPProxyPort)
	offset := 4

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.MPTCPIPv4Address.To4())
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.MPTCPIPv6Address.To16())
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *MPTCPAddressInformationFields) MarshalLen() int {
	l := 1 + 1 + 2
	if f.HasIPv4() {
		l += 4
	}
	if f.HasIPv6() {
		l += 16
	}

	return l
}
