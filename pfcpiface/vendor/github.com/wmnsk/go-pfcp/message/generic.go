// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"fmt"

	"github.com/wmnsk/go-pfcp/ie"
)

// Generic is a Generic formed PFCP Header and its IEs above.
type Generic struct {
	*Header
	IEs []*ie.IE
}

// NewGeneric creates a new Generic.
func NewGeneric(msgType uint8, seid uint64, seq uint32, ies ...*ie.IE) *Generic {
	m := &Generic{
		Header: NewHeader(
			1, 0, 0, 1,
			msgType, seid, seq, 0,
			nil,
		),
		IEs: ies,
	}
	m.SetLength()

	return m
}

// NewGenericWithoutSEID creates a new Generic.
func NewGenericWithoutSEID(msgType uint8, seq uint32, ies ...*ie.IE) *Generic {
	m := &Generic{
		Header: NewHeader(
			1, 0, 0, 0,
			msgType, 0, seq, 0,
			nil,
		),
		IEs: ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a Generic.
func (m *Generic) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *Generic) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
	for _, ie := range m.IEs {
		if ie == nil {
			continue
		}
		if err := ie.MarshalTo(m.Header.Payload[offset:]); err != nil {
			return err
		}
		offset += ie.MarshalLen()
	}

	m.Header.SetLength()
	return m.Header.MarshalTo(b)
}

// ParseGeneric decodes a given byte sequence as a Generic.
func ParseGeneric(b []byte) (*Generic, error) {
	m := &Generic{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a Generic.
func (m *Generic) UnmarshalBinary(b []byte) error {
	var err error
	m.Header, err = ParseHeader(b)
	if err != nil {
		return err
	}
	if len(m.Header.Payload) < 2 {
		return nil
	}

	m.IEs, err = ie.ParseMultiIEs(m.Header.Payload)
	if err != nil {
		return err
	}
	return nil
}

// MarshalLen returns the serial length of Data.
func (m *Generic) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	for _, ie := range m.IEs {
		if ie == nil {
			continue
		}
		l += ie.MarshalLen()
	}

	return l
}

// SetLength sets the length in Length field.
func (m *Generic) SetLength() {
	l := m.Header.MarshalLen() - len(m.Header.Payload) - 4
	for _, ie := range m.IEs {
		l += ie.MarshalLen()
	}
	m.Header.Length = uint16(l)
}

// MessageTypeName returns the name of protocol.
func (m *Generic) MessageTypeName() string {
	return fmt.Sprintf("Unknown (%d)", m.Header.Type)
}

// SEID returns the SEID in uint64.
func (m *Generic) SEID() uint64 {
	return m.Header.seid()
}

// AddIE add IEs to Generic type of PFCP message and update Length field.
func (m *Generic) AddIE(ies ...*ie.IE) {
	m.IEs = append(m.IEs, ies...)
	m.SetLength()
}
