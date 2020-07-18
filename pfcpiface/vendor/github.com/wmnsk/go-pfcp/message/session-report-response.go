// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionReportResponse is a SessionReportResponse formed PFCP Header and its IEs above.
type SessionReportResponse struct {
	*Header
	Cause                   *ie.IE
	OffendingIE             *ie.IE
	UpdateBAR               *ie.IE
	PFCPSRRspFlags          *ie.IE
	CPFSEID                 *ie.IE
	N4UFTEID                *ie.IE
	AlternativeSMFIPAddress *ie.IE
	IEs                     []*ie.IE
}

// NewSessionReportResponse creates a new SessionReportResponse.
func NewSessionReportResponse(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionReportResponse {
	m := &SessionReportResponse{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionReportResponse, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.Cause:
			m.Cause = i
		case ie.OffendingIE:
			m.OffendingIE = i
		case ie.UpdateBARWithinSessionReportResponse:
			m.UpdateBAR = i
		case ie.PFCPSRRspFlags:
			m.PFCPSRRspFlags = i
		case ie.FSEID:
			m.CPFSEID = i
		case ie.FTEID:
			m.N4UFTEID = i
		case ie.AlternativeSMFIPAddress:
			m.AlternativeSMFIPAddress = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionReportResponse.
func (m *SessionReportResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionReportResponse) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
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
	if i := m.UpdateBAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PFCPSRRspFlags; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CPFSEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.N4UFTEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.AlternativeSMFIPAddress; i != nil {
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

// ParseSessionReportResponse decodes a given byte sequence as a SessionReportResponse.
func ParseSessionReportResponse(b []byte) (*SessionReportResponse, error) {
	m := &SessionReportResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionReportResponse.
func (m *SessionReportResponse) UnmarshalBinary(b []byte) error {
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
		case ie.Cause:
			m.Cause = i
		case ie.OffendingIE:
			m.OffendingIE = i
		case ie.UpdateBARWithinSessionReportResponse:
			m.UpdateBAR = i
		case ie.PFCPSRRspFlags:
			m.PFCPSRRspFlags = i
		case ie.FSEID:
			m.CPFSEID = i
		case ie.FTEID:
			m.N4UFTEID = i
		case ie.AlternativeSMFIPAddress:
			m.AlternativeSMFIPAddress = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionReportResponse) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.Cause; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OffendingIE; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateBAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PFCPSRRspFlags; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CPFSEID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.N4UFTEID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.AlternativeSMFIPAddress; i != nil {
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
func (m *SessionReportResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionReportResponse) MessageTypeName() string {
	return "Session Report Response"
}

// SEID returns the SEID in uint64.
func (m *SessionReportResponse) SEID() uint64 {
	return m.Header.seid()
}
