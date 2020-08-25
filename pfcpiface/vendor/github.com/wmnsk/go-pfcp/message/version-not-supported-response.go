// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// VersionNotSupportedResponse is a VersionNotSupportedResponse formed PFCP Header and its IEs above.
type VersionNotSupportedResponse struct {
	*Header
	IEs []*ie.IE
}

// NewVersionNotSupportedResponse creates a new VersionNotSupportedResponse.
func NewVersionNotSupportedResponse(seq uint32, ies ...*ie.IE) *VersionNotSupportedResponse {
	m := &VersionNotSupportedResponse{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeVersionNotSupportedResponse, 0, seq, 0,
			nil,
		),
		IEs: ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a VersionNotSupportedResponse.
func (m *VersionNotSupportedResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *VersionNotSupportedResponse) MarshalTo(b []byte) error {
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

// ParseVersionNotSupportedResponse decodes a given byte sequence as a VersionNotSupportedResponse.
func ParseVersionNotSupportedResponse(b []byte) (*VersionNotSupportedResponse, error) {
	m := &VersionNotSupportedResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a VersionNotSupportedResponse.
func (m *VersionNotSupportedResponse) UnmarshalBinary(b []byte) error {
	var err error
	m.Header, err = ParseHeader(b)
	if err != nil {
		return err
	}
	if len(m.Header.Payload) < 2 {
		return nil
	}

	ies, err := ie.ParseMultiIEs(m.Header.Payload)
	if err != nil {
		return err
	}

	m.IEs = append(m.IEs, ies...)

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *VersionNotSupportedResponse) MarshalLen() int {
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
func (m *VersionNotSupportedResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *VersionNotSupportedResponse) MessageTypeName() string {
	return "Version Not Supported Response"
}

// SEID returns the SEID in uint64.
func (m *VersionNotSupportedResponse) SEID() uint64 {
	return m.Header.seid()
}
