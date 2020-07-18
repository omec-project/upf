// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"net"
)

// NewRemoteGTPUPeer creates a new RemoteGTPUPeer IE.
func NewRemoteGTPUPeer(flags uint8, v4, v6 string, di uint8, ni string) *IE {
	fields := NewRemoteGTPUPeerFields(flags, v4, v6, di, ni)
	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(RemoteGTPUPeer, b)
}

// RemoteGTPUPeer returns RemoteGTPUPeer in *RemoteGTPUPeerFields if the type of IE matches.
func (i *IE) RemoteGTPUPeer() (*RemoteGTPUPeerFields, error) {
	switch i.Type {
	case RemoteGTPUPeer:
		fields, err := ParseRemoteGTPUPeerFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case UserPlanePathFailureReport:
		ies, err := i.UserPlanePathFailureReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RemoteGTPUPeer {
				return x.RemoteGTPUPeer()
			}
		}
		return nil, ErrIENotFound
	case UserPlanePathRecoveryReport:
		ies, err := i.UserPlanePathRecoveryReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RemoteGTPUPeer {
				return x.RemoteGTPUPeer()
			}
		}
		return nil, ErrIENotFound
	case GTPUPathQoSControlInformation:
		ies, err := i.GTPUPathQoSControlInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RemoteGTPUPeer {
				return x.RemoteGTPUPeer()
			}
		}
		return nil, ErrIENotFound
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == RemoteGTPUPeer {
				return x.RemoteGTPUPeer()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasNI reports whether an IE has NI bit.
func (i *IE) HasNI() bool {
	if i.Type != RemoteGTPUPeer {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has4thBit(i.Payload[0])
}

// HasDI reports whether an IE has DI bit.
func (i *IE) HasDI() bool {
	if i.Type != RemoteGTPUPeer {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has3rdBit(i.Payload[0])
}

// RemoteGTPUPeerFields represents a fields contained in RemoteGTPUPeer IE.
type RemoteGTPUPeerFields struct {
	Flags                uint8
	IPv4Address          net.IP
	IPv6Address          net.IP
	DILength             uint16
	DestinationInterface uint8
	NILength             uint16
	NetworkInstance      string
}

// NewRemoteGTPUPeerFields creates a new RemoteGTPUPeerFields.
func NewRemoteGTPUPeerFields(flags uint8, v4, v6 string, di uint8, ni string) *RemoteGTPUPeerFields {
	f := &RemoteGTPUPeerFields{Flags: flags}

	if has2ndBit(flags) {
		f.IPv4Address = net.ParseIP(v4).To4()
	}

	if has1stBit(flags) {
		f.IPv6Address = net.ParseIP(v6).To16()
	}

	if has3rdBit(flags) {
		f.DILength = 1 // DI is half-octet long
		f.DestinationInterface = di & 0x0f
	}

	if has4thBit(flags) {
		f.NILength = uint16(len(ni))
		f.NetworkInstance = ni
	}

	return f
}

// ParseRemoteGTPUPeerFields parses b into RemoteGTPUPeerFields.
func ParseRemoteGTPUPeerFields(b []byte) (*RemoteGTPUPeerFields, error) {
	f := &RemoteGTPUPeerFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *RemoteGTPUPeerFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if has2ndBit(f.Flags) {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.IPv4Address = net.IP(b[offset : offset+4]).To4()
		offset += 4
	}

	if has1stBit(f.Flags) {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Address = net.IP(b[offset : offset+16]).To16()
		offset += 16
	}

	if has3rdBit(f.Flags) {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		f.DILength = binary.BigEndian.Uint16(b[offset : offset+2])
		offset += 2

		if l < offset+int(f.DILength) {
			return io.ErrUnexpectedEOF
		}
		f.DestinationInterface = b[offset]
		offset += int(f.DILength)
	}

	if has4thBit(f.Flags) {
		if l < offset+2 {
			return io.ErrUnexpectedEOF
		}
		f.NILength = binary.BigEndian.Uint16(b[offset : offset+2])
		offset += 2

		if l < offset+int(f.NILength) {
			return io.ErrUnexpectedEOF
		}
		f.NetworkInstance = string(b[offset : offset+int(f.NILength)])
	}

	return nil
}

// Marshal returns the serialized bytes of RemoteGTPUPeerFields.
func (f *RemoteGTPUPeerFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *RemoteGTPUPeerFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if has2ndBit(f.Flags) {
		copy(b[offset:offset+4], f.IPv4Address)
		offset += 4
	}

	if has1stBit(f.Flags) {
		copy(b[offset:offset+16], f.IPv6Address)
		offset += 16
	}

	if has3rdBit(f.Flags) {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.DILength)
		offset += 2
		b[offset] = f.DestinationInterface
		offset += int(f.DILength)
	}

	if has4thBit(f.Flags) {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.NILength)
		offset += 2
		copy(b[offset:offset+int(f.NILength)], []byte(f.NetworkInstance))
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *RemoteGTPUPeerFields) MarshalLen() int {
	l := 1
	if has2ndBit(f.Flags) && !has5thBit(f.Flags) {
		l += 4
	}
	if has1stBit(f.Flags) && !has5thBit(f.Flags) {
		l += 16
	}
	if has3rdBit(f.Flags) {
		l += 2 + int(f.DILength)
	}
	if has4thBit(f.Flags) {
		l += 2 + int(f.NILength)
	}

	return l
}
