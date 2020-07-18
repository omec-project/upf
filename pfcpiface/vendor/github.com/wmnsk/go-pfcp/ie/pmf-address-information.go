// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"net"
)

// NewPMFAddressInformation creates a new PMFAddressInformation IE.
func NewPMFAddressInformation(v4, v6 net.IP, port1, port2 uint16, mac1, mac2 net.HardwareAddr) *IE {
	fields := NewPMFAddressInformationFields(v4, v6, port1, port2, mac1, mac2)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(PMFAddressInformation, b)
}

// PMFAddressInformation returns PMFAddressInformation in structured format if the type of IE matches.
func (i *IE) PMFAddressInformation() (*PMFAddressInformationFields, error) {
	switch i.Type {
	case PMFAddressInformation:
		fields, err := ParsePMFAddressInformationFields(i.Payload)
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
			if x.Type == PMFParameters {
				return x.PMFAddressInformation()
			}
		}
		return nil, ErrIENotFound
	case PMFParameters:
		ies, err := i.PMFParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PMFAddressInformation {
				return x.PMFAddressInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// PMFAddressInformationFields represents a fields contained in PMFAddressInformation IE.
type PMFAddressInformationFields struct {
	Flags                         uint8
	PMFIPv4Address                net.IP
	PMFIPv6Address                net.IP
	PMFPortFor3GPPAccess          uint16
	PMFPortForNon3GPPAccess       uint16
	PMFMACAddressFor3GPPAccess    net.HardwareAddr
	PMFMACAddressForNon3GPPAccess net.HardwareAddr
}

// NewPMFAddressInformationFields creates a new NewPMFAddressInformationFields.
func NewPMFAddressInformationFields(v4, v6 net.IP, port1, port2 uint16, mac1, mac2 net.HardwareAddr) *PMFAddressInformationFields {
	f := &PMFAddressInformationFields{
		Flags:                         0x00,
		PMFPortFor3GPPAccess:          port1,
		PMFPortForNon3GPPAccess:       port2,
		PMFMACAddressFor3GPPAccess:    mac1,
		PMFMACAddressForNon3GPPAccess: mac2,
	}

	if v4 != nil {
		f.SetIPv4Flag()
		f.PMFIPv4Address = v4
	}
	if v6 != nil {
		f.SetIPv6Flag()
		f.PMFIPv6Address = v6
	}

	return f
}

// HasMAC reports whether MAC flag is set.
func (f *PMFAddressInformationFields) HasMAC() bool {
	return has3rdBit(f.Flags)
}

// SetMACFlag sets MAC flag in PMFAddressInformation.
func (f *PMFAddressInformationFields) SetMACFlag() {
	f.Flags |= 0x04
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *PMFAddressInformationFields) HasIPv6() bool {
	return has2ndBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in PMFAddressInformation.
func (f *PMFAddressInformationFields) SetIPv6Flag() {
	f.Flags |= 0x02
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *PMFAddressInformationFields) HasIPv4() bool {
	return has1stBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in PMFAddressInformation.
func (f *PMFAddressInformationFields) SetIPv4Flag() {
	f.Flags |= 0x01
}

// ParsePMFAddressInformationFields parses b into PMFAddressInformationFields.
func ParsePMFAddressInformationFields(b []byte) (*PMFAddressInformationFields, error) {
	f := &PMFAddressInformationFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *PMFAddressInformationFields) UnmarshalBinary(b []byte) error {
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
		f.PMFIPv4Address = net.IP(b[offset : offset+4])
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.PMFIPv6Address = net.IP(b[offset : offset+16])
		offset += 16
	}

	if l < offset+2 {
		return io.ErrUnexpectedEOF
	}
	f.PMFPortFor3GPPAccess = binary.BigEndian.Uint16(b[offset : offset+2])
	offset += 2

	if l < offset+2 {
		return io.ErrUnexpectedEOF
	}
	f.PMFPortForNon3GPPAccess = binary.BigEndian.Uint16(b[offset : offset+2])
	offset += 2

	if l < offset+6 {
		return io.ErrUnexpectedEOF
	}
	copy(f.PMFMACAddressFor3GPPAccess, b[offset:offset+6])
	offset += 6

	if l < offset+6 {
		return io.ErrUnexpectedEOF
	}
	copy(f.PMFMACAddressForNon3GPPAccess, b[offset:offset+6])

	return nil
}

// Marshal returns the serialized bytes of PMFAddressInformationFields.
func (f *PMFAddressInformationFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *PMFAddressInformationFields) MarshalTo(b []byte) error {
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
		copy(b[offset:offset+4], f.PMFIPv4Address.To4())
		offset += 4
	}

	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.PMFIPv6Address.To16())
		offset += 16
	}

	if l < offset+2 {
		return io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint16(b[offset:offset+2], f.PMFPortFor3GPPAccess)
	offset += 2

	if l < offset+2 {
		return io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint16(b[offset:offset+2], f.PMFPortForNon3GPPAccess)
	offset += 2

	if l < offset+6 {
		return io.ErrUnexpectedEOF
	}
	copy(b[offset:offset+6], f.PMFMACAddressFor3GPPAccess)
	offset += 6

	if l < offset+6 {
		return io.ErrUnexpectedEOF
	}
	copy(b[offset:offset+6], f.PMFMACAddressForNon3GPPAccess)

	return nil
}

// MarshalLen returns field length in integer.
func (f *PMFAddressInformationFields) MarshalLen() int {
	l := 1 + 2 + 2 + 6 + 6
	if f.HasIPv4() {
		l += 4
	}
	if f.HasIPv6() {
		l += 16
	}
	return l
}
