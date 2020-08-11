// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionDeletionResponse is a SessionDeletionResponse formed PFCP Header and its IEs above.
type SessionDeletionResponse struct {
	*Header
	IEs []*ie.IE
}

// NewSessionDeletionResponse creates a new SessionDeletionResponse.
func NewSessionDeletionResponse(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionDeletionResponse {
	m := &SessionDeletionResponse{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionDeletionResponse, seid, seq, pri,
			nil,
		),
		IEs: ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a SessionDeletionResponse.
func (m *SessionDeletionResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionDeletionResponse) MarshalTo(b []byte) error {
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

// ParseSessionDeletionResponse decodes a given byte sequence as a SessionDeletionResponse.
func ParseSessionDeletionResponse(b []byte) (*SessionDeletionResponse, error) {
	m := &SessionDeletionResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionDeletionResponse.
func (m *SessionDeletionResponse) UnmarshalBinary(b []byte) error {
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
func (m *SessionDeletionResponse) MarshalLen() int {
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
func (m *SessionDeletionResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionDeletionResponse) MessageTypeName() string {
	return "Session Deletion Response"
}

// SEID returns the SEID in uint64.
func (m *SessionDeletionResponse) SEID() uint64 {
	return m.Header.seid()
}
