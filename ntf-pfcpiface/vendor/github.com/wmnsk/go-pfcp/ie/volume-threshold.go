// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewVolumeThreshold creates a new VolumeThreshold IE.
func NewVolumeThreshold(flags uint8, tvol, uvol, dvol uint64) *IE {
	fields := NewVolumeThresholdFields(flags, tvol, uvol, dvol)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(VolumeThreshold, b)
}

// VolumeThreshold returns VolumeThreshold in structured format if the type of IE matches.
func (i *IE) VolumeThreshold() (*VolumeThresholdFields, error) {
	switch i.Type {
	case VolumeThreshold:
		f, err := ParseVolumeThresholdFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return f, nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == VolumeThreshold {
				return x.VolumeThreshold()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == VolumeThreshold {
				return x.VolumeThreshold()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// VolumeThresholdFields represents a fields contained in VolumeThreshold IE.
type VolumeThresholdFields struct {
	Flags          uint8
	TotalVolume    uint64
	UplinkVolume   uint64
	DownlinkVolume uint64
}

// NewVolumeThresholdFields creates a new NewVolumeThresholdFields.
func NewVolumeThresholdFields(flags uint8, tvol, uvol, dvol uint64) *VolumeThresholdFields {
	f := &VolumeThresholdFields{Flags: flags}

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
func (f *VolumeThresholdFields) HasDLVOL() bool {
	return has3rdBit(f.Flags)
}

// SetDLVOLFlag sets DLVOL flag in VolumeThreshold.
func (f *VolumeThresholdFields) SetDLVOLFlag() {
	f.Flags |= 0x04
}

// HasULVOL reports whether ULVOL flag is set.
func (f *VolumeThresholdFields) HasULVOL() bool {
	return has2ndBit(f.Flags)
}

// SetULVOLFlag sets ULVOL flag in VolumeThreshold.
func (f *VolumeThresholdFields) SetULVOLFlag() {
	f.Flags |= 0x02
}

// HasTOVOL reports whether TOVOL flag is set.
func (f *VolumeThresholdFields) HasTOVOL() bool {
	return has1stBit(f.Flags)
}

// SetTOVOLFlag sets TOVOL flag in VolumeThreshold.
func (f *VolumeThresholdFields) SetTOVOLFlag() {
	f.Flags |= 0x01
}

// ParseVolumeThresholdFields parses b into VolumeThresholdFields.
func ParseVolumeThresholdFields(b []byte) (*VolumeThresholdFields, error) {
	f := &VolumeThresholdFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *VolumeThresholdFields) UnmarshalBinary(b []byte) error {
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

// Marshal returns the serialized bytes of VolumeThresholdFields.
func (f *VolumeThresholdFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *VolumeThresholdFields) MarshalTo(b []byte) error {
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
func (f *VolumeThresholdFields) MarshalLen() int {
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
