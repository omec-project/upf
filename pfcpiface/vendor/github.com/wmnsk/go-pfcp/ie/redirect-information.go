// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewRedirectInformation creates a new RedirectInformation IE.
func NewRedirectInformation(addrType uint8, addrs ...string) *IE {
	fields := NewRedirectInformationFields(addrType, addrs...)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(RedirectInformation, b)
}

// RedirectInformation returns RedirectInformation in structured format if the type of IE matches.
func (i *IE) RedirectInformation() (*RedirectInformationFields, error) {
	switch i.Type {
	case RedirectInformation:
		f, err := ParseRedirectInformationFields(i.Payload)
		if err != nil {
			return nil, err
		}
		return f, nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedirectInformation {
				return x.RedirectInformation()
			}
		}
		return nil, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RedirectInformation {
				return x.RedirectInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// RedirectAddressType definitions.
const (
	RedirectAddrIPv4        uint8 = 0
	RedirectAddrIPv6        uint8 = 1
	RedirectAddrURL         uint8 = 2
	RedirectAddrSIPURI      uint8 = 3
	RedirectAddrIPv4AndIPv6 uint8 = 4
)

// RedirectInformationFields represents a fields contained in RedirectInformation IE.
type RedirectInformationFields struct {
	RedirectAddressType        uint8 // half octet
	ServerAddrLength           uint16
	RedirectServerAddress      string
	OtherServerAddrLength      uint16
	OtherRedirectServerAddress string
}

// NewRedirectInformationFields creates a new NewRedirectInformationFields.
//
// You can put multiple addrs here, but the second one is used only when addrType is
// RedirectAddrIPv4AndIPv6. Third - nth addrs will never be used.
func NewRedirectInformationFields(addrType uint8, addrs ...string) *RedirectInformationFields {
	if len(addrs) < 1 {
		return nil
	}

	f := &RedirectInformationFields{
		RedirectAddressType:   addrType,
		ServerAddrLength:      uint16(len(addrs[0])),
		RedirectServerAddress: addrs[0],
	}

	if len(addrs) >= 2 {
		f.OtherServerAddrLength = uint16(len(addrs[1]))
		f.OtherRedirectServerAddress = addrs[1]
	}

	return f
}

// ParseRedirectInformationFields parses b into RedirectInformationFields.
func ParseRedirectInformationFields(b []byte) (*RedirectInformationFields, error) {
	f := &RedirectInformationFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *RedirectInformationFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 3 {
		return io.ErrUnexpectedEOF
	}

	f.RedirectAddressType = b[0]
	offset := 1

	f.ServerAddrLength = binary.BigEndian.Uint16(b[offset : offset+2])
	offset += 2

	if l < offset+int(f.ServerAddrLength) {
		return io.ErrUnexpectedEOF
	}
	f.RedirectServerAddress = string(b[offset : offset+int(f.ServerAddrLength)])

	f.OtherServerAddrLength = binary.BigEndian.Uint16(b[1:3])
	if f.OtherServerAddrLength != 0 {
		offset += 2

		if l < offset+int(f.OtherServerAddrLength) {
			return io.ErrUnexpectedEOF
		}
		f.OtherRedirectServerAddress = string(b[offset : offset+int(f.OtherServerAddrLength)])
	}

	return nil
}

// Marshal returns the serialized bytes of RedirectInformationFields.
func (f *RedirectInformationFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *RedirectInformationFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.RedirectAddressType
	offset := 1

	if l < offset+int(f.ServerAddrLength) {
		return io.ErrUnexpectedEOF
	}

	binary.BigEndian.PutUint16(b[1:3], f.ServerAddrLength)
	offset += 2

	copy(b[offset:offset+int(f.ServerAddrLength)], []byte(f.RedirectServerAddress))
	offset += int(f.ServerAddrLength)

	if f.OtherRedirectServerAddress == "" {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.OtherServerAddrLength)
		return nil
	}

	if l < offset+int(f.OtherServerAddrLength) {
		return io.ErrUnexpectedEOF
	}

	binary.BigEndian.PutUint16(b[offset:offset+2], f.OtherServerAddrLength)
	offset += 2

	copy(b[offset:offset+int(f.OtherServerAddrLength)], []byte(f.OtherRedirectServerAddress))
	return nil
}

// MarshalLen returns field length in integer.
func (f *RedirectInformationFields) MarshalLen() int {
	return 3 + int(f.ServerAddrLength) + 2 + int(f.OtherServerAddrLength)
}
