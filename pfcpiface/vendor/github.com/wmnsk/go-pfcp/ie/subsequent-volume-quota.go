// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSubsequentVolumeQuota creates a new SubsequentVolumeQuota IE.
func NewSubsequentVolumeQuota(flags uint8, tvol, uvol, dvol uint64) *IE {
	fields := NewSubsequentVolumeQuotaFields(flags, tvol, uvol, dvol)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(SubsequentVolumeQuota, b)
}

// SubsequentVolumeQuota returns SubsequentVolumeQuota in structured format if the type of IE matches.
func (i *IE) SubsequentVolumeQuota() (*SubsequentVolumeQuotaFields, error) {
	switch i.Type {
	case SubsequentVolumeQuota:
		fields, err := ParseSubsequentVolumeQuotaFields(i.Payload)
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
			if x.Type == SubsequentVolumeQuota {
				return x.SubsequentVolumeQuota()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SubsequentVolumeQuota {
				return x.SubsequentVolumeQuota()
			}
		}
		return nil, ErrIENotFound
	case AdditionalMonitoringTime:
		ies, err := i.AdditionalMonitoringTime()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == SubsequentVolumeQuota {
				return x.SubsequentVolumeQuota()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// SubsequentVolumeQuotaFields represents a fields contained in SubsequentVolumeQuota IE.
type SubsequentVolumeQuotaFields struct {
	Flags          uint8
	TotalVolume    uint64
	UplinkVolume   uint64
	DownlinkVolume uint64
}

// NewSubsequentVolumeQuotaFields creates a new NewSubsequentVolumeQuotaFields.
func NewSubsequentVolumeQuotaFields(flags uint8, tvol, uvol, dvol uint64) *SubsequentVolumeQuotaFields {
	f := &SubsequentVolumeQuotaFields{Flags: flags}

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
func (f *SubsequentVolumeQuotaFields) HasDLVOL() bool {
	return has3rdBit(f.Flags)
}

// SetDLVOLFlag sets DLVOL flag in SubsequentVolumeQuota.
func (f *SubsequentVolumeQuotaFields) SetDLVOLFlag() {
	f.Flags |= 0x04
}

// HasULVOL reports whether ULVOL flag is set.
func (f *SubsequentVolumeQuotaFields) HasULVOL() bool {
	return has2ndBit(f.Flags)
}

// SetULVOLFlag sets ULVOL flag in SubsequentVolumeQuota.
func (f *SubsequentVolumeQuotaFields) SetULVOLFlag() {
	f.Flags |= 0x02
}

// HasTOVOL reports whether TOVOL flag is set.
func (f *SubsequentVolumeQuotaFields) HasTOVOL() bool {
	return has1stBit(f.Flags)
}

// SetTOVOLFlag sets TOVOL flag in SubsequentVolumeQuota.
func (f *SubsequentVolumeQuotaFields) SetTOVOLFlag() {
	f.Flags |= 0x01
}

// ParseSubsequentVolumeQuotaFields parses b into SubsequentVolumeQuotaFields.
func ParseSubsequentVolumeQuotaFields(b []byte) (*SubsequentVolumeQuotaFields, error) {
	f := &SubsequentVolumeQuotaFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *SubsequentVolumeQuotaFields) UnmarshalBinary(b []byte) error {
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

// Marshal returns the serialized bytes of SubsequentVolumeQuotaFields.
func (f *SubsequentVolumeQuotaFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *SubsequentVolumeQuotaFields) MarshalTo(b []byte) error {
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
func (f *SubsequentVolumeQuotaFields) MarshalLen() int {
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
