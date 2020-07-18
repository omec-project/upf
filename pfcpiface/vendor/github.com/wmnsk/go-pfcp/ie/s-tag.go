// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSTAG creates a new STAG IE.
func NewSTAG(flags, pcp, deiFlag uint8, cvid uint16) *IE {
	fields := NewSTAGFields(flags, pcp, deiFlag, cvid)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(STAG, b)
}

// STAG returns STAG in structured format if the type of IE matches.
func (i *IE) STAG() (*STAGFields, error) {
	switch i.Type {
	case STAG:
		fields, err := ParseSTAGFields(i.Payload)
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
			if x.Type == EthernetPacketFilter {
				return x.STAG()
			}
		}
		return nil, ErrIENotFound
	case EthernetPacketFilter:
		ies, err := i.EthernetPacketFilter()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == STAG {
				return x.STAG()
			}
		}
		return nil, ErrIENotFound
	case MACAddressesDetected:
		x, err := i.MACAddressesDetected()
		if err != nil {
			return nil, err
		}
		return ParseSTAGFields(x.STAG)
	case MACAddressesRemoved:
		x, err := i.MACAddressesRemoved()
		if err != nil {
			return nil, err
		}
		return ParseSTAGFields(x.STAG)
	case EthernetContextInformation:
		ies, err := i.EthernetContextInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == MACAddressesDetected {
				return x.STAG()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// STAGFields represents a fields contained in STAG IE.
type STAGFields struct {
	Flags   uint8
	PCP     uint8  // 3 bit
	DEIFlag uint8  // 1 bit
	CVID    uint16 // 12 bit
}

// NewSTAGFields creates a new NewSTAGFields.
func NewSTAGFields(flags, pcp, deiFlag uint8, cvid uint16) *STAGFields {
	return &STAGFields{
		Flags:   flags,
		PCP:     pcp,
		DEIFlag: deiFlag,
		CVID:    cvid,
	}
}

// HasVID reports whether VID flag is set.
func (f *STAGFields) HasVID() bool {
	return has3rdBit(f.Flags)
}

// SetVIDFlag sets VID flag in STAG.
func (f *STAGFields) SetVIDFlag() {
	f.Flags |= 0x04
}

// HasDEI reports whether DEI flag is set.
func (f *STAGFields) HasDEI() bool {
	return has2ndBit(f.Flags)
}

// SetDEIFlag sets DEI flag in STAG.
func (f *STAGFields) SetDEIFlag() {
	f.Flags |= 0x02
}

// HasPCP reports whether PCP flag is set.
func (f *STAGFields) HasPCP() bool {
	return has1stBit(f.Flags)
}

// SetPCPFlag sets PCP flag in STAG.
func (f *STAGFields) SetPCPFlag() {
	f.Flags |= 0x01
}

// ParseSTAGFields parses b into STAGFields.
func ParseSTAGFields(b []byte) (*STAGFields, error) {
	f := &STAGFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *STAGFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 3 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if f.HasPCP() {
		f.PCP = b[offset] & 0x07
	}

	if f.HasDEI() {
		f.DEIFlag = (b[offset] >> 3) & 0x01
	}

	if f.HasVID() {
		f.CVID = binary.BigEndian.Uint16(b[offset:offset+2]) & 0xf0ff
	}

	return nil
}

// Marshal returns the serialized bytes of STAGFields.
func (f *STAGFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *STAGFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	binary.BigEndian.PutUint16(b[offset:offset+2], ((f.CVID<<4)&0xf000|f.CVID&0xff)|(uint16(f.DEIFlag<<3|f.PCP)<<8))
	return nil
}

// MarshalLen returns field length in integer.
func (f *STAGFields) MarshalLen() int {
	return 3
}
