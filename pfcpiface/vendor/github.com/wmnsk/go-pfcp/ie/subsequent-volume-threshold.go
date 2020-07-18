// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSubsequentVolumeThreshold creates a new SubsequentVolumeThreshold IE.
func NewSubsequentVolumeThreshold(flags uint8, tvol, uvol, dvol uint64) *IE {
	fields := NewSubsequentVolumeThresholdFields(flags, tvol, uvol, dvol)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(SubsequentVolumeThreshold, b)
}

// SubsequentVolumeThreshold returns SubsequentVolumeThreshold in structured format if the type of IE matches.
func (i *IE) SubsequentVolumeThreshold() (*SubsequentVolumeThresholdFields, error) {
	switch i.Type {
	case SubsequentVolumeThreshold:
		fields, err := ParseSubsequentVolumeThresholdFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SubsequentVolumeThreshold {
				return x.SubsequentVolumeThreshold()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SubsequentVolumeThreshold {
				return x.SubsequentVolumeThreshold()
			}
		}
		return nil, ErrIENotFound
	case AdditionalMonitoringTime:
		ies, err := i.AdditionalMonitoringTime()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SubsequentVolumeThreshold {
				return x.SubsequentVolumeThreshold()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// SubsequentVolumeThresholdFields represents a fields contained in SubsequentVolumeThreshold IE.
type SubsequentVolumeThresholdFields struct {
	Flags          uint8
	TotalVolume    uint64
	UplinkVolume   uint64
	DownlinkVolume uint64
}

// NewSubsequentVolumeThresholdFields creates a new NewSubsequentVolumeThresholdFields.
func NewSubsequentVolumeThresholdFields(flags uint8, tvol, uvol, dvol uint64) *SubsequentVolumeThresholdFields {
	f := &SubsequentVolumeThresholdFields{Flags: flags}

	if f.HasTOVOL() {
		f.TotalVolume = tvol
	}

	if f.HasULVOL() {
		f.UplinkVolume = uvol
	}

	if f.HasDLVOL() {
		f.DownlinkVolume = dvol
	}

	return f
}

// HasDLVOL reports whether DLVOL flag is set.
func (f *SubsequentVolumeThresholdFields) HasDLVOL() bool {
	return has3rdBit(f.Flags)
}

// SetDLVOLFlag sets DLVOL flag in SubsequentVolumeThreshold.
func (f *SubsequentVolumeThresholdFields) SetDLVOLFlag() {
	f.Flags |= 0x04
}

// HasULVOL reports whether ULVOL flag is set.
func (f *SubsequentVolumeThresholdFields) HasULVOL() bool {
	return has2ndBit(f.Flags)
}

// SetULVOLFlag sets ULVOL flag in SubsequentVolumeThreshold.
func (f *SubsequentVolumeThresholdFields) SetULVOLFlag() {
	f.Flags |= 0x02
}

// HasTOVOL reports whether TOVOL flag is set.
func (f *SubsequentVolumeThresholdFields) HasTOVOL() bool {
	return has1stBit(f.Flags)
}

// SetTOVOLFlag sets TOVOL flag in SubsequentVolumeThreshold.
func (f *SubsequentVolumeThresholdFields) SetTOVOLFlag() {
	f.Flags |= 0x01
}

// ParseSubsequentVolumeThresholdFields parses b into SubsequentVolumeThresholdFields.
func ParseSubsequentVolumeThresholdFields(b []byte) (*SubsequentVolumeThresholdFields, error) {
	f := &SubsequentVolumeThresholdFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *SubsequentVolumeThresholdFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasTOVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.TotalVolume = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	if f.HasULVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.UplinkVolume = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	if f.HasDLVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.DownlinkVolume = binary.BigEndian.Uint64(b[offset : offset+8])
	}

	return nil
}

// Marshal returns the serialized bytes of SubsequentVolumeThresholdFields.
func (f *SubsequentVolumeThresholdFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *SubsequentVolumeThresholdFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasTOVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.TotalVolume)
		offset += 8
	}

	if f.HasULVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.UplinkVolume)
		offset += 8
	}

	if f.HasDLVOL() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.DownlinkVolume)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *SubsequentVolumeThresholdFields) MarshalLen() int {
	l := 1
	if f.HasTOVOL() {
		l += 8
	}
	if f.HasULVOL() {
		l += 8
	}
	if f.HasDLVOL() {
		l += 8
	}

	return l
}
