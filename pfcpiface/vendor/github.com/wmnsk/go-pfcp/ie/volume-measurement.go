// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewVolumeMeasurement creates a new VolumeMeasurement IE.
func NewVolumeMeasurement(flags uint8, tvol, uvol, dvol, tpkt, upkt, dpkt uint64) *IE {
	fields := NewVolumeMeasurementFields(flags, tvol, uvol, dvol, tpkt, upkt, dpkt)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(VolumeMeasurement, b)
}

// VolumeMeasurement returns VolumeMeasurement in structured format if the type of IE matches.
func (i *IE) VolumeMeasurement() (*VolumeMeasurementFields, error) {
	switch i.Type {
	case VolumeMeasurement:
		fields, err := ParseVolumeMeasurementFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == VolumeMeasurement {
				return x.VolumeMeasurement()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// VolumeMeasurementFields represents a fields contained in VolumeMeasurement IE.
type VolumeMeasurementFields struct {
	Flags                   uint8
	TotalVolume             uint64
	UplinkVolume            uint64
	DownlinkVolume          uint64
	TotalNumberOfPackets    uint64
	UplinkNumberOfPackets   uint64
	DownlinkNumberOfPackets uint64
}

// NewVolumeMeasurementFields creates a new NewVolumeMeasurementFields.
func NewVolumeMeasurementFields(flags uint8, tvol, uvol, dvol, tpkt, upkt, dpkt uint64) *VolumeMeasurementFields {
	f := &VolumeMeasurementFields{Flags: flags}

	if f.HasTOVOL() {
		f.TotalVolume = tvol
	}

	if f.HasULVOL() {
		f.UplinkVolume = uvol
	}

	if f.HasDLVOL() {
		f.DownlinkVolume = dvol
	}

	if f.HasTONOP() {
		f.TotalNumberOfPackets = tpkt
	}

	if f.HasULNOP() {
		f.UplinkNumberOfPackets = upkt
	}

	if f.HasDLNOP() {
		f.DownlinkNumberOfPackets = dpkt
	}

	return f
}

// HasDLNOP reports whether DLNOP flag is set.
func (f *VolumeMeasurementFields) HasDLNOP() bool {
	return has6thBit(f.Flags)
}

// SetDLNOPFlag sets DLNOP flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetDLNOPFlag() {
	f.Flags |= 0x20
}

// HasULNOP reports whether ULNOP flag is set.
func (f *VolumeMeasurementFields) HasULNOP() bool {
	return has5thBit(f.Flags)
}

// SetULNOPFlag sets ULNOP flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetULNOPFlag() {
	f.Flags |= 0x10
}

// HasTONOP reports whether TONOP flag is set.
func (f *VolumeMeasurementFields) HasTONOP() bool {
	return has4thBit(f.Flags)
}

// SetTONOPFlag sets TONOP flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetTONOPFlag() {
	f.Flags |= 0x08
}

// HasDLVOL reports whether DLVOL flag is set.
func (f *VolumeMeasurementFields) HasDLVOL() bool {
	return has3rdBit(f.Flags)
}

// SetDLVOLFlag sets DLVOL flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetDLVOLFlag() {
	f.Flags |= 0x04
}

// HasULVOL reports whether ULVOL flag is set.
func (f *VolumeMeasurementFields) HasULVOL() bool {
	return has2ndBit(f.Flags)
}

// SetULVOLFlag sets ULVOL flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetULVOLFlag() {
	f.Flags |= 0x02
}

// HasTOVOL reports whether TOVOL flag is set.
func (f *VolumeMeasurementFields) HasTOVOL() bool {
	return has1stBit(f.Flags)
}

// SetTOVOLFlag sets TOVOL flag in VolumeMeasurement.
func (f *VolumeMeasurementFields) SetTOVOLFlag() {
	f.Flags |= 0x01
}

// ParseVolumeMeasurementFields parses b into VolumeMeasurementFields.
func ParseVolumeMeasurementFields(b []byte) (*VolumeMeasurementFields, error) {
	f := &VolumeMeasurementFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *VolumeMeasurementFields) UnmarshalBinary(b []byte) error {
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
		offset += 8
	}

	if f.HasTONOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.TotalNumberOfPackets = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	if f.HasULNOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.UplinkNumberOfPackets = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	if f.HasDLNOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		f.DownlinkNumberOfPackets = binary.BigEndian.Uint64(b[offset : offset+8])
	}

	return nil
}

// Marshal returns the serialized bytes of VolumeMeasurementFields.
func (f *VolumeMeasurementFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *VolumeMeasurementFields) MarshalTo(b []byte) error {
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
		offset += 8
	}

	if f.HasTONOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.TotalNumberOfPackets)
		offset += 8
	}

	if f.HasULNOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.UplinkNumberOfPackets)
		offset += 8
	}

	if f.HasDLNOP() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint64(b[offset:offset+8], f.DownlinkNumberOfPackets)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *VolumeMeasurementFields) MarshalLen() int {
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
	if f.HasTONOP() {
		l += 8
	}
	if f.HasULNOP() {
		l += 8
	}
	if f.HasDLNOP() {
		l += 8
	}

	return l
}
