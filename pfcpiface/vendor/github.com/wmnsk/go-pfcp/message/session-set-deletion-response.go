// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionSetDeletionResponse is a SessionSetDeletionResponse formed PFCP Header and its IEs above.
type SessionSetDeletionResponse struct {
	*Header
	NodeID      *ie.IE
	Cause       *ie.IE
	OffendingIE *ie.IE
	IEs         []*ie.IE
}

// NewSessionSetDeletionResponse creates a new SessionSetDeletionResponse.
func NewSessionSetDeletionResponse(seq uint32, id, cause, offending *ie.IE, ies ...*ie.IE) *SessionSetDeletionResponse {
	m := &SessionSetDeletionResponse{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeSessionSetDeletionResponse, 0, seq, 0,
			nil,
		),
		NodeID:      id,
		Cause:       cause,
		OffendingIE: offending,
		IEs:         ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a SessionSetDeletionResponse.
func (m *SessionSetDeletionResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionSetDeletionResponse) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
	if i := m.NodeID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.Cause; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.OffendingIE; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}

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

// ParseSessionSetDeletionResponse decodes a given byte sequence as a SessionSetDeletionResponse.
func ParseSessionSetDeletionResponse(b []byte) (*SessionSetDeletionResponse, error) {
	m := &SessionSetDeletionResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionSetDeletionResponse.
func (m *SessionSetDeletionResponse) UnmarshalBinary(b []byte) error {
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

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.Cause:
			m.Cause = i
		case ie.OffendingIE:
			m.OffendingIE = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionSetDeletionResponse) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.Cause; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OffendingIE; i != nil {
		l += i.MarshalLen()
	}

	for _, ie := range m.IEs {
		if ie == nil {
			continue
		}
		l += ie.MarshalLen()
	}

	return l
}

// SetLength sets the length in Length field.
func (m *SessionSetDeletionResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionSetDeletionResponse) MessageTypeName() string {
	return "Node Report Response"
}

// SEID returns the SEID in uint64.
func (m *SessionSetDeletionResponse) SEID() uint64 {
	return m.Header.seid()
}
