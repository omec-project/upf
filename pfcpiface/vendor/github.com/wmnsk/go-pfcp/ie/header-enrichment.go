// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// HeaderType definitions.
const (
	HeaderTypeHTTP uint8 = 0
)

// NewHeaderEnrichment creates a new HeaderEnrichment IE.
func NewHeaderEnrichment(typ uint8, name, value string) *IE {
	fields := NewHeaderEnrichmentFields(typ, name, value)
	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(HeaderEnrichment, b)
}

// HeaderEnrichment returns HeaderEnrichment in *HeaderEnrichmentFields if the type of IE matches.
func (i *IE) HeaderEnrichment() (*HeaderEnrichmentFields, error) {
	switch i.Type {
	case HeaderEnrichment:
		f, err := ParseHeaderEnrichmentFields(i.Payload)
		if err != nil {
			return nil, err
		}
		return f, nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == HeaderEnrichment {
				return x.HeaderEnrichment()
			}
		}
		return nil, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == HeaderEnrichment {
				return x.HeaderEnrichment()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HeaderEnrichmentFields represents a fields contained in HeaderEnrichment IE.
type HeaderEnrichmentFields struct {
	Flags            uint8
	HeaderType       uint8
	NameLength       uint8
	HeaderFieldName  string
	ValueLength      uint8
	HeaderFieldValue string
}

// NewHeaderEnrichmentFields creates a new HeaderEnrichmentFields.
func NewHeaderEnrichmentFields(typ uint8, name, value string) *HeaderEnrichmentFields {
	return &HeaderEnrichmentFields{
		HeaderType:       typ,
		NameLength:       uint8(len([]byte(name))),
		HeaderFieldName:  name,
		ValueLength:      uint8(len([]byte(value))),
		HeaderFieldValue: value,
	}
}

// ParseHeaderEnrichmentFields parses b into HeaderEnrichmentFields.
func ParseHeaderEnrichmentFields(b []byte) (*HeaderEnrichmentFields, error) {
	f := &HeaderEnrichmentFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *HeaderEnrichmentFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	f.HeaderType = b[0]
	f.NameLength = b[1]
	offset := 2

	if l < offset+int(f.NameLength) {
		return io.ErrUnexpectedEOF
	}
	f.HeaderFieldName = string(b[offset : offset+int(f.NameLength)])
	offset += int(f.NameLength)

	if l < offset+1 {
		return io.ErrUnexpectedEOF
	}
	f.ValueLength = b[offset]
	offset++

	if l < offset+int(f.ValueLength) {
		return io.ErrUnexpectedEOF
	}
	f.HeaderFieldValue = string(b[offset : offset+int(f.ValueLength)])

	return nil
}

// Marshal returns the serialized bytes of HeaderEnrichmentFields.
func (f *HeaderEnrichmentFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *HeaderEnrichmentFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 2 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.HeaderType
	b[1] = f.NameLength
	offset := 2

	copy(b[offset:offset+int(f.NameLength)], []byte(f.HeaderFieldName))
	offset += int(f.NameLength)

	b[offset] = f.ValueLength
	offset++

	copy(b[offset:offset+int(f.ValueLength)], []byte(f.HeaderFieldValue))

	return nil
}

// MarshalLen returns field length in integer.
func (f *HeaderEnrichmentFields) MarshalLen() int {
	l := 3
	l += len([]byte(f.HeaderFieldName))
	l += len([]byte(f.HeaderFieldValue))

	return l
}
