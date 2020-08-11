// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
)

// NewUserPlaneIPResourceInformation creates a new UserPlaneIPResourceInformation IE.
func NewUserPlaneIPResourceInformation(flags uint8, tRange uint8, v4, v6, ni string, si uint8) *IE {
	fields := NewUserPlaneIPResourceInformationFields(flags, tRange, v4, v6, ni, si)
	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(UserPlaneIPResourceInformation, b)
}

// UserPlaneIPResourceInformation returns UserPlaneIPResourceInformation in *UserPlaneIPResourceInformationFields if the type of IE matches.
func (i *IE) UserPlaneIPResourceInformation() (*UserPlaneIPResourceInformationFields, error) {
	if i.Type != UserPlaneIPResourceInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	f, err := ParseUserPlaneIPResourceInformationFields(i.Payload)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// TEIDRI returns the number of bits that are used to partition the TEID range.
func (i *IE) TEIDRI() int {
	if i.Type != UserPlaneIPResourceInformation {
		return 0
	}

	return int(i.Payload[0]>>2) & 0x07
}

// HasASSONI reports whether an IE has ASSONI bit.
func (i *IE) HasASSONI() bool {
	if i.Type != UserPlaneIPResourceInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has6thBit(i.Payload[0])
}

// HasASSOSI reports whether an IE has ASSOSI bit.
func (i *IE) HasASSOSI() bool {
	if i.Type != UserPlaneIPResourceInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has7thBit(i.Payload[0])
}

// UserPlaneIPResourceInformationFields represents a fields contained in UserPlaneIPResourceInformation IE.
type UserPlaneIPResourceInformationFields struct {
	Flags           uint8
	TEIDRange       uint8
	IPv4Address     net.IP
	IPv6Address     net.IP
	NetworkInstance string
	SourceInterface uint8
}

// NewUserPlaneIPResourceInformationFields creates a new UserPlaneIPResourceInformationFields.
func NewUserPlaneIPResourceInformationFields(flags uint8, tRange uint8, v4, v6, ni string, si uint8) *UserPlaneIPResourceInformationFields {
	f := &UserPlaneIPResourceInformationFields{Flags: flags}

	if (flags>>2)&0x07 != 0 {
		f.TEIDRange = tRange
	}

	if has1stBit(flags) {
		f.IPv4Address = net.ParseIP(v4).To4()
	}

	if has2ndBit(flags) {
		f.IPv6Address = net.ParseIP(v6).To16()
	}

	if has6thBit(flags) {
		f.NetworkInstance = ni
	}

	if has7thBit(flags) {
		f.SourceInterface = si
	}

	return f
}

// ParseUserPlaneIPResourceInformationFields parses b into UserPlaneIPResourceInformationFields.
func ParseUserPlaneIPResourceInformationFields(b []byte) (*UserPlaneIPResourceInformationFields, error) {
	f := &UserPlaneIPResourceInformationFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into UserPlaneIPResourceInformationFields.
func (f *UserPlaneIPResourceInformationFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if (f.Flags>>2)&0x07 != 0 {
		if l < offset+1 {
			return io.ErrUnexpectedEOF
		}
		f.TEIDRange = b[offset]
		offset += 1
	}

	if has1stBit(f.Flags) {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.IPv4Address = net.IP(b[offset : offset+4]).To4()
		offset += 4
	}

	if has2ndBit(f.Flags) {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Address = net.IP(b[offset : offset+16]).To16()
		offset += 16
	}

	if has6thBit(f.Flags) {
		n := l
		if has7thBit(f.Flags) {
			f.SourceInterface = b[n] & 0x0f
			n--
		}

		if l < offset+n {
			return io.ErrUnexpectedEOF
		}
		f.NetworkInstance = string(b[offset:n])
		return nil
	}

	if has7thBit(f.Flags) {
		f.SourceInterface = b[offset] & 0x0f
	}

	return nil
}

// Marshal returns the serialized bytes of UserPlaneIPResourceInformationFields.
func (f *UserPlaneIPResourceInformationFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *UserPlaneIPResourceInformationFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if (f.Flags>>2)&0x07 != 0 {
		b[offset] = f.TEIDRange
		offset += 1
	}

	if has1stBit(f.Flags) {
		copy(b[offset:offset+4], f.IPv4Address)
		offset += 4
	}

	if has2ndBit(f.Flags) {
		copy(b[offset:offset+16], f.IPv6Address)
		offset += 16
	}
	if has6thBit(f.Flags) {
		n := len([]byte(f.NetworkInstance))
		copy(b[offset:offset+n], []byte(f.NetworkInstance))
		offset += n
	}

	if has7thBit(f.Flags) {
		b[offset] = f.SourceInterface
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *UserPlaneIPResourceInformationFields) MarshalLen() int {
	l := 1
	if (f.Flags>>2)&0x07 != 0 {
		l++
	}
	if has1stBit(f.Flags) {
		l += 4
	}
	if has2ndBit(f.Flags) {
		l += 16
	}
	if has6thBit(f.Flags) {
		l += len([]byte(f.NetworkInstance))
	}
	if has7thBit(f.Flags) {
		l++
	}

	return l
}
