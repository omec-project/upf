// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionDeletionRequest is a SessionDeletionRequest formed PFCP Header and its IEs above.
type SessionDeletionRequest struct {
	*Header
	IEs []*ie.IE
}

// NewSessionDeletionRequest creates a new SessionDeletionRequest.
func NewSessionDeletionRequest(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionDeletionRequest {
	m := &SessionDeletionRequest{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionDeletionRequest, seid, seq, pri,
			nil,
		),
		IEs: ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a SessionDeletionRequest.
func (m *SessionDeletionRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionDeletionRequest) MarshalTo(b []byte) error {
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

// ParseSessionDeletionRequest decodes a given byte sequence as a SessionDeletionRequest.
func ParseSessionDeletionRequest(b []byte) (*SessionDeletionRequest, error) {
	m := &SessionDeletionRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionDeletionRequest.
func (m *SessionDeletionRequest) UnmarshalBinary(b []byte) error {
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
func (m *SessionDeletionRequest) MarshalLen() int {
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
func (m *SessionDeletionRequest) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionDeletionRequest) MessageTypeName() string {
	return "Session Deletion Request"
}

// SEID returns the SEID in uint64.
func (m *SessionDeletionRequest) SEID() uint64 {
	return m.Header.seid()
}
