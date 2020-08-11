// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

// NewFTEID creates a new FTEID IE.
func NewFTEID(teid uint32, v4, v6 net.IP, chid []byte) *IE {
	fields := NewFTEIDFields(teid, v4, v6, chid)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(FTEID, b)
}

// FTEID returns FTEID in structured format if the type of IE matches.
func (i *IE) FTEID() (*FTEIDFields, error) {
	switch i.Type {
	case FTEID:
		fields, err := ParseFTEIDFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			switch i.Type {
			case FTEID, RedundantTransmissionParameters:
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case CreatedPDR:
		ies, err := i.CreatedPDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case ErrorIndicationReport:
		ies, err := i.ErrorIndicationReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case CreatedTrafficEndpoint:
		return nil, errors.New("cannot determine which value to return. Use LocalFTEID or LocalFTEIDN instead")
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case RedundantTransmissionParameters:
		ies, err := i.RedundantTransmissionParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	case UpdatedPDR:
		ies, err := i.UpdatedPDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FTEID {
				return x.FTEID()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// FTEIDFields represents a fields contained in FTEID IE.
type FTEIDFields struct {
	Flags       uint8
	TEID        uint32
	IPv4Address net.IP
	IPv6Address net.IP
	ChooseID    []byte
}

// NewFTEIDFields creates a new NewFTEIDFields.
func NewFTEIDFields(teid uint32, v4, v6 net.IP, chid []byte) *FTEIDFields {
	var fields *FTEIDFields
	if chid != nil {
		fields = &FTEIDFields{
			IPv4Address: v4,
			IPv6Address: v6,
			ChooseID:    chid,
		}
		fields.SetChIDFlag()

	} else {
		fields = &FTEIDFields{
			TEID:        teid,
			IPv4Address: v4,
			IPv6Address: v6,
		}
	}

	if v4 != nil {
		fields.SetIPv4Flag()
	}
	if v6 != nil {
		fields.SetIPv6Flag()
	}

	return fields
}

// HasChID reports whether CHID flag is set.
func (f *FTEIDFields) HasChID() bool {
	return has4thBit(f.Flags)
}

// SetChIDFlag sets CHID flag in FTEID.
func (f *FTEIDFields) SetChIDFlag() {
	f.Flags |= 0x08
}

// HasCh reports whether CH flag is set.
func (f *FTEIDFields) HasCh() bool {
	return has3rdBit(f.Flags)
}

// SetChFlag sets CH flag in FTEID.
func (f *FTEIDFields) SetChFlag() {
	f.Flags |= 0x04
}

// HasIPv6 reports whether IPv6 flag is set.
func (f *FTEIDFields) HasIPv6() bool {
	return has2ndBit(f.Flags)
}

// SetIPv6Flag sets IPv6 flag in FTEID.
func (f *FTEIDFields) SetIPv6Flag() {
	f.Flags |= 0x02
}

// HasIPv4 reports whether IPv4 flag is set.
func (f *FTEIDFields) HasIPv4() bool {
	return has1stBit(f.Flags)
}

// SetIPv4Flag sets IPv4 flag in FTEID.
func (f *FTEIDFields) SetIPv4Flag() {
	f.Flags |= 0x01
}

// ParseFTEIDFields parses b into FTEIDFields.
func ParseFTEIDFields(b []byte) (*FTEIDFields, error) {
	f := &FTEIDFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *FTEIDFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasChID() || f.HasCh() {
		if f.HasIPv4() {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			f.IPv4Address = net.IP(b[offset : offset+4])
			offset += 4
		}
		if f.HasIPv6() {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			f.IPv6Address = net.IP(b[offset : offset+16])
			offset += 16
		}

		if l <= offset {
			return nil
		}

		f.ChooseID = b[offset:]
		return nil
	}

	if l < offset+4 {
		return nil
	}
	f.TEID = binary.BigEndian.Uint32(b[offset : offset+4])
	offset += 4

	if f.HasIPv4() {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		f.IPv4Address = net.IP(b[offset : offset+4])
		offset += 4
	}
	if f.HasIPv6() {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		f.IPv6Address = net.IP(b[offset : offset+16])
	}

	return nil
}

// Marshal returns the serialized bytes of FTEIDFields.
func (f *FTEIDFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *FTEIDFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if f.HasChID() || f.HasCh() {
		if f.IPv4Address != nil {
			if l < offset+4 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+4], f.IPv4Address.To4())
			offset += 4
		}
		if f.IPv6Address != nil {
			if l < offset+16 {
				return io.ErrUnexpectedEOF
			}
			copy(b[offset:offset+16], f.IPv6Address.To16())
			offset += 16
		}

		copy(b[offset:], f.ChooseID)
		return nil
	}

	if l < offset+4 {
		return io.ErrUnexpectedEOF
	}
	binary.BigEndian.PutUint32(b[offset:offset+4], f.TEID)
	offset += 4

	if f.IPv4Address != nil {
		if l < offset+4 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+4], f.IPv4Address.To4())
		offset += 4
	}
	if f.IPv6Address != nil {
		if l < offset+16 {
			return io.ErrUnexpectedEOF
		}
		copy(b[offset:offset+16], f.IPv6Address.To16())
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *FTEIDFields) MarshalLen() int {
	l := 1
	if f.IPv4Address != nil {
		l += 4
	}
	if f.IPv6Address != nil {
		l += 16
	}

	if f.HasChID() || f.HasCh() {
		l += len(f.ChooseID)
		return l
	}

	return l + 4
}
