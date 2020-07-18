// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewQoSMonitoringMeasurement creates a new QoSMonitoringMeasurement IE.
func NewQoSMonitoringMeasurement(flags uint8, dl, ul, rp uint32) *IE {
	fields := NewQoSMonitoringMeasurementFields(flags, dl, ul, rp)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(QoSMonitoringMeasurement, b)
}

// QoSMonitoringMeasurement returns QoSMonitoringMeasurement in structured format if the type of IE matches.
func (i *IE) QoSMonitoringMeasurement() (*QoSMonitoringMeasurementFields, error) {
	switch i.Type {
	case QoSMonitoringMeasurement:
		fields, err := ParseQoSMonitoringMeasurementFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case QoSMonitoringReport:
		ies, err := i.QoSMonitoringReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == QoSMonitoringMeasurement {
				return x.QoSMonitoringMeasurement()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// QoSMonitoringMeasurementFields represents a fields contained in QoSMonitoringMeasurement IE.
type QoSMonitoringMeasurementFields struct {
	Flags                uint8
	DownlinkPacketDelay  uint32
	UplinkPacketDelay    uint32
	RoundTripPacketDelay uint32
}

// NewQoSMonitoringMeasurementFields creates a new NewQoSMonitoringMeasurementFields.
func NewQoSMonitoringMeasurementFields(flags uint8, dl, ul, rp uint32) *QoSMonitoringMeasurementFields {
	return &QoSMonitoringMeasurementFields{
		Flags:                flags,
		DownlinkPacketDelay:  dl,
		UplinkPacketDelay:    ul,
		RoundTripPacketDelay: rp,
	}
}

// HasPLMF reports whether PLMF flag is set.
func (f *QoSMonitoringMeasurementFields) HasPLMF() bool {
	return has4thBit(f.Flags)
}

// SetPLMFFlag sets PLMF flag in QoSMonitoringMeasurement.
func (f *QoSMonitoringMeasurementFields) SetPLMFFlag() {
	f.Flags |= 0x08
}

// HasRP reports whether RP flag is set.
func (f *QoSMonitoringMeasurementFields) HasRP() bool {
	return has3rdBit(f.Flags)
}

// SetRPFlag sets RP flag in QoSMonitoringMeasurement.
func (f *QoSMonitoringMeasurementFields) SetRPFlag() {
	f.Flags |= 0x04
}

// HasUL reports whether UL flag is set.
func (f *QoSMonitoringMeasurementFields) HasUL() bool {
	return has2ndBit(f.Flags)
}

// SetULFlag sets UL flag in QoSMonitoringMeasurement.
func (f *QoSMonitoringMeasurementFields) SetULFlag() {
	f.Flags |= 0x02
}

// HasDL reports whether DL flag is set.
func (f *QoSMonitoringMeasurementFields) HasDL() bool {
	return has1stBit(f.Flags)
}

// SetDLFlag sets DL flag in QoSMonitoringMeasurement.
func (f *QoSMonitoringMeasurementFields) SetDLFlag() {
	f.Flags |= 0x01
}

// ParseQoSMonitoringMeasurementFields parses b into QoSMonitoringMeasurementFields.
func ParseQoSMonitoringMeasurementFields(b []byte) (*QoSMonitoringMeasurementFields, error) {
	f := &QoSMonitoringMeasurementFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *QoSMonitoringMeasurementFields) UnmarshalBinary(b []byte) error {
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
		f.DownlinkPacketDelay = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}

	if f.HasUL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.UplinkPacketDelay = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}

	if f.HasRP() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.RoundTripPacketDelay = binary.BigEndian.Uint32(b[offset : offset+4])
	}

	return nil
}

// Marshal returns the serialized bytes of QoSMonitoringMeasurementFields.
func (f *QoSMonitoringMeasurementFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *QoSMonitoringMeasurementFields) MarshalTo(b []byte) error {
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
		binary.BigEndian.PutUint32(b[offset:offset+4], f.DownlinkPacketDelay)
		offset += 4
	}

	if f.HasUL() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.UplinkPacketDelay)
		offset += 4
	}

	if f.HasRP() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		binary.BigEndian.PutUint32(b[offset:offset+4], f.RoundTripPacketDelay)
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *QoSMonitoringMeasurementFields) MarshalLen() int {
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
