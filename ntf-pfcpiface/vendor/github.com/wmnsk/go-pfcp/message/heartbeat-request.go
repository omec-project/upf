// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// HeartbeatRequest is a HeartbeatRequest formed PFCP Header and its IEs above.
type HeartbeatRequest struct {
	*Header
	RecoveryTimeStamp *ie.IE
	SourceIPAddress   *ie.IE
	IEs               []*ie.IE
}

// NewHeartbeatRequest creates a new HeartbeatRequest.
func NewHeartbeatRequest(seq uint32, ts, ip *ie.IE, ies ...*ie.IE) *HeartbeatRequest {
	m := &HeartbeatRequest{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeHeartbeatRequest, 0, seq, 0,
			nil,
		),
		RecoveryTimeStamp: ts,
		SourceIPAddress:   ip,
		IEs:               ies,
	}
	m.SetLength()

	return m
}

// Marshal returns the byte sequence generated from a HeartbeatRequest.
func (m *HeartbeatRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *HeartbeatRequest) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
	if i := m.RecoveryTimeStamp; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.SourceIPAddress; i != nil {
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

// ParseHeartbeatRequest decodes a given byte sequence as a HeartbeatRequest.
func ParseHeartbeatRequest(b []byte) (*HeartbeatRequest, error) {
	m := &HeartbeatRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a HeartbeatRequest.
func (m *HeartbeatRequest) UnmarshalBinary(b []byte) error {
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
		case ie.RecoveryTimeStamp:
			m.RecoveryTimeStamp = i
		case ie.SourceIPAddress:
			m.SourceIPAddress = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *HeartbeatRequest) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.RecoveryTimeStamp; i != nil {
		l += i.MarshalLen()
	}
	if i := m.SourceIPAddress; i != nil {
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
func (m *HeartbeatRequest) SetLength() {
	l := m.Header.MarshalLen() - len(m.Header.Payload) - 4

	if i := m.RecoveryTimeStamp; i != nil {
		l += i.MarshalLen()
	}
	if i := m.SourceIPAddress; i != nil {
		l += i.MarshalLen()
	}

	for _, ie := range m.IEs {
		l += ie.MarshalLen()
	}
	m.Header.Length = uint16(l)
}

// MessageTypeName returns the name of protocol.
func (m *HeartbeatRequest) MessageTypeName() string {
	return "Heartbeat Request"
}

// SEID returns the SEID in uint64.
func (m *HeartbeatRequest) SEID() uint64 {
	return m.Header.seid()
}
