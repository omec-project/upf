// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"

	"github.com/wmnsk/go-pfcp/internal/utils"
)

// NewUserID creates a new UserID IE.
func NewUserID(flags uint8, imsi, imei, msisdn, nai string) *IE {
	fields := NewUserIDFields(flags, imsi, imei, msisdn, nai)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(UserID, b)
}

// UserID returns UserID in structured format if the type of IE matches.
func (i *IE) UserID() (*UserIDFields, error) {
	if i.Type != UserID {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	s, err := ParseUserIDFields(i.Payload)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UserIDFields represents a fields contained in UserID IE.
type UserIDFields struct {
	Flags        uint8
	IMSILength   uint8
	IMSI         string
	IMEILength   uint8
	IMEI         string
	MSISDNLength uint8
	MSISDN       string
	NAILength    uint8
	NAI          string
}

// NewUserIDFields creates a new NewUserIDFields.
func NewUserIDFields(flags uint8, imsi, imei, msisdn, nai string) *UserIDFields {
	f := &UserIDFields{
		Flags: flags,
	}

	if has1stBit(flags) {
		v, err := utils.StrToSwappedBytes(imsi, "f")
		if err != nil {
			return nil
		}
		f.IMSILength = uint8(len(v))
		f.IMSI = imsi
	}

	if has2ndBit(flags) {
		v, err := utils.StrToSwappedBytes(imei, "f")
		if err != nil {
			return nil
		}
		f.IMEILength = uint8(len(v))
		f.IMEI = imei
	}

	if has3rdBit(flags) {
		v, err := utils.StrToSwappedBytes(msisdn, "f")
		if err != nil {
			return nil
		}
		f.MSISDNLength = uint8(len(v))
		f.MSISDN = msisdn
	}

	if has4thBit(flags) {
		f.NAILength = uint8(len([]byte(nai)))
		f.NAI = nai
	}

	return f
}

// ParseUserIDFields parses b into UserIDFields.
func ParseUserIDFields(b []byte) (*UserIDFields, error) {
	f := &UserIDFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *UserIDFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 1

	if has1stBit(f.Flags) {
		if l < offset+1 {
			return io.ErrUnexpectedEOF
		}
		f.IMSILength = b[offset]
		offset++

		if l < offset+int(f.IMSILength) {
			return io.ErrUnexpectedEOF
		}
		f.IMSI = utils.SwappedBytesToStr(b[offset:offset+int(f.IMSILength)], true)
		offset += int(f.IMSILength)
	}

	if has2ndBit(f.Flags) {
		if l < offset+1 {
			return io.ErrUnexpectedEOF
		}
		f.IMEILength = b[offset]
		offset++

		if l < offset+int(f.IMEILength) {
			return io.ErrUnexpectedEOF
		}
		f.IMEI = utils.SwappedBytesToStr(b[offset:offset+int(f.IMEILength)], true)
		offset += int(f.IMEILength)
	}

	if has3rdBit(f.Flags) {
		if l < offset+1 {
			return io.ErrUnexpectedEOF
		}
		f.MSISDNLength = b[offset]
		offset++

		if l < offset+int(f.MSISDNLength) {
			return io.ErrUnexpectedEOF
		}
		f.MSISDN = utils.SwappedBytesToStr(b[offset:offset+int(f.MSISDNLength)], true)
		offset += int(f.MSISDNLength)
	}

	if has4thBit(f.Flags) {
		if l < offset+1 {
			return io.ErrUnexpectedEOF
		}
		f.NAILength = b[offset]
		offset++

		if l < offset+int(f.NAILength) {
			return io.ErrUnexpectedEOF
		}
		f.NAI = string(b[offset : offset+int(f.NAILength)])
	}

	return nil
}

// Marshal returns the serialized bytes of UserIDFields.
func (f *UserIDFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *UserIDFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 1

	if has1stBit(f.Flags) {
		b[offset] = f.IMSILength
		offset++

		v, err := utils.StrToSwappedBytes(f.IMSI, "f")
		if err != nil {
			return err
		}
		copy(b[offset:offset+int(f.IMSILength)], v)
		offset += int(f.IMSILength)
	}

	if has2ndBit(f.Flags) {
		b[offset] = f.IMEILength
		offset++

		v, err := utils.StrToSwappedBytes(f.IMEI, "f")
		if err != nil {
			return err
		}
		copy(b[offset:offset+int(f.IMEILength)], v)
		offset += int(f.IMEILength)
	}

	if has3rdBit(f.Flags) {
		b[offset] = f.MSISDNLength
		offset++

		v, err := utils.StrToSwappedBytes(f.MSISDN, "f")
		if err != nil {
			return err
		}
		copy(b[offset:offset+int(f.MSISDNLength)], v)
		offset += int(f.MSISDNLength)
	}

	if has1stBit(f.Flags) {
		b[offset] = f.NAILength
		offset++

		copy(b[offset:offset+int(f.NAILength)], []byte(f.NAI))
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *UserIDFields) MarshalLen() int {
	l := 1
	if has1stBit(f.Flags) {
		l += 1 + int(f.IMSILength)
	}
	if has2ndBit(f.Flags) {
		l += 1 + int(f.IMEILength)
	}
	if has3rdBit(f.Flags) {
		l += 1 + int(f.MSISDNLength)
	}
	if has4thBit(f.Flags) {
		l += 1 + int(f.NAILength)
	}

	return l
}
