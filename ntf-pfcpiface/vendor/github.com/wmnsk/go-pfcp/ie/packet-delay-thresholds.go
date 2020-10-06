// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewPacketDelayThresholds creates a new PacketDelayThresholds IE.
func NewPacketDelayThresholds(flags uint8, dl, ul, rp uint32) *IE {
	fields := NewPacketDelayThresholdsFields(flags, dl, ul, rp)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(PacketDelayThresholds, b)
}

// PacketDelayThresholds returns PacketDelayThresholds in structured format if the type of IE matches.
func (i *IE) PacketDelayThresholds() (*PacketDelayThresholdsFields, error) {
	switch i.Type {
	case PacketDelayThresholds:
		fields, err := ParsePacketDelayThresholdsFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketDelayThresholds {
				return x.PacketDelayThresholds()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// PacketDelayThresholdsFields represents a fields contained in PacketDelayThresholds IE.
type PacketDelayThresholdsFields struct {
	Flags                          uint8
	DownlinkPacketDelayThresholds  uint32
	UplinkPacketDelayThresholds    uint32
	RoundTripPacketDelayThresholds uint32
}

// NewPacketDelayThresholdsFields creates a new NewPacketDelayThresholdsFields.
func NewPacketDelayThresholdsFields(flags uint8, dl, ul, rp uint32) *PacketDelayThresholdsFields {
	return &PacketDelayThresholdsFields{
		Flags:                          flags,
		DownlinkPacketDelayThresholds:  dl,
		UplinkPacketDelayThresholds:    ul,
		RoundTripPacketDelayThresholds: rp,
	}
}

// HasRP reports whether RP flag is set.
func (f *PacketDelayThresholdsFields) HasRP() bool {
	return has3rdBit(f.Flags)
}

// SetRPFlag sets RP flag in PacketDelayThresholds.
func (f *PacketDelayThresholdsFields) SetRPFlag() {
	f.Flags |= 0x04
}

// HasUL reports whether UL flag is set.
func (f *PacketDelayThresholdsFields) HasUL() bool {
	return has2ndBit(f.Flags)
}

// SetULFlag sets UL flag in PacketDelayThresholds.
func (f *PacketDelayThresholdsFields) SetULFlag() {
	f.Flags |= 0x02
}

// HasDL reports whether DL flag is set.
func (f *PacketDelayThresholdsFields) HasDL() bool {
	return has1stBit(f.Flags)
}

// SetDLFlag sets DL flag in PacketDelayThresholds.
func (f *PacketDelayThresholdsFields) SetDLFlag() {
	f.Flags |= 0x01
}

// ParsePacketDelayThresholdsFields parses b into PacketDelayThresholdsFields.
func ParsePacketDelayThresholdsFields(b []byte) (*PacketDelayThresholdsFields, error) {
	f := &PacketDelayThresholdsFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *PacketDelayThresholdsFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasDL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.DownlinkPacketDelayThresholds = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}

	if f.HasUL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.UplinkPacketDelayThresholds = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}

	if f.HasRP() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.RoundTripPacketDelayThresholds = binary.BigEndian.Uint32(b[offset : offset+4])
	}

	return nil
}

// Marshal returns the serialized bytes of PacketDelayThresholdsFields.
func (f *PacketDelayThresholdsFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *PacketDelayThresholdsFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasDL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.DownlinkPacketDelayThresholds)
		offset += 4
	}

	if f.HasUL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.UplinkPacketDelayThresholds)
		offset += 4
	}

	if f.HasRP() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.RoundTripPacketDelayThresholds)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *PacketDelayThresholdsFields) MarshalLen() int {
	l := 1
	if f.HasDL() {
		l += 4
	}
	if f.HasUL() {
		l += 4
	}
	if f.HasRP() {
		l += 4
	}
	return l
}
