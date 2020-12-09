// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionEstablishmentResponse is a SessionEstablishmentResponse formed PFCP Header and its IEs above.
//
// TODO: add RDS configuration information IE.
type SessionEstablishmentResponse struct {
	*Header
	NodeID                     *ie.IE
	Cause                      *ie.IE
	OffendingIE                *ie.IE
	UPFSEID                    *ie.IE
	CreatedPDR                 []*ie.IE
	LoadControlInformation     *ie.IE
	OverloadControlInformation *ie.IE
	FQCSID                     *ie.IE
	FailedRuleID               *ie.IE
	CreatedTrafficEndpoint     []*ie.IE
	CreatedBridgeInfoForTSC    *ie.IE
	ATSSSControlParameters     *ie.IE
	IEs                        []*ie.IE
}

// NewSessionEstablishmentResponse creates a new SessionEstablishmentResponse.
func NewSessionEstablishmentResponse(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionEstablishmentResponse {
	m := &SessionEstablishmentResponse{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionEstablishmentResponse, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.Cause:
			m.Cause = i
		case ie.OffendingIE:
			m.OffendingIE = i
		case ie.FSEID:
			m.UPFSEID = i
		case ie.CreatedPDR:
			m.CreatedPDR = append(m.CreatedPDR, i)
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.FailedRuleID:
			m.FailedRuleID = i
		case ie.CreatedTrafficEndpoint:
			m.CreatedTrafficEndpoint = append(m.CreatedTrafficEndpoint, i)
		case ie.CreatedBridgeInfoForTSC:
			m.CreatedBridgeInfoForTSC = i
		case ie.ATSSSControlParameters:
			m.ATSSSControlParameters = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionEstablishmentResponse.
func (m *SessionEstablishmentResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionEstablishmentResponse) MarshalTo(b []byte) error {
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
	if i := m.UPFSEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreatedPDR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.LoadControlInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.OverloadControlInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.FQCSID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.FailedRuleID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreatedTrafficEndpoint {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreatedBridgeInfoForTSC; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.ATSSSControlParameters; i != nil {
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

// ParseSessionEstablishmentResponse decodes a given byte sequence as a SessionEstablishmentResponse.
func ParseSessionEstablishmentResponse(b []byte) (*SessionEstablishmentResponse, error) {
	m := &SessionEstablishmentResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionEstablishmentResponse.
func (m *SessionEstablishmentResponse) UnmarshalBinary(b []byte) error {
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
		case ie.FSEID:
			m.UPFSEID = i
		case ie.CreatedPDR:
			m.CreatedPDR = append(m.CreatedPDR, i)
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.FailedRuleID:
			m.FailedRuleID = i
		case ie.CreatedTrafficEndpoint:
			m.CreatedTrafficEndpoint = append(m.CreatedTrafficEndpoint, i)
		case ie.CreatedBridgeInfoForTSC:
			m.CreatedBridgeInfoForTSC = i
		case ie.ATSSSControlParameters:
			m.ATSSSControlParameters = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionEstablishmentResponse) MarshalLen() int {
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
	if i := m.UPFSEID; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreatedPDR {
		l += i.MarshalLen()
	}
	if i := m.LoadControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OverloadControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.FQCSID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.FailedRuleID; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreatedTrafficEndpoint {
		l += i.MarshalLen()
	}
	if i := m.CreatedBridgeInfoForTSC; i != nil {
		l += i.MarshalLen()
	}
	if i := m.ATSSSControlParameters; i != nil {
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
func (m *SessionEstablishmentResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionEstablishmentResponse) MessageTypeName() string {
	return "Session Establishment Response"
}

// SEID returns the SEID in uint64.
func (m *SessionEstablishmentResponse) SEID() uint64 {
	return m.Header.seid()
}
