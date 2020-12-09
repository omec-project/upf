// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionEstablishmentRequest is a SessionEstablishmentRequest formed PFCP Header and its IEs above.
//
// TODO: add S-NSSAI and Provide RDS configuration information.
type SessionEstablishmentRequest struct {
	*Header
	NodeID                         *ie.IE
	CPFSEID                        *ie.IE
	CreatePDR                      []*ie.IE
	CreateFAR                      []*ie.IE
	CreateURR                      []*ie.IE
	CreateQER                      []*ie.IE
	CreateBAR                      *ie.IE
	CreateTrafficEndpoint          []*ie.IE
	PDNType                        *ie.IE
	FQCSID                         *ie.IE
	UserPlaneInactivityTimer       *ie.IE
	UserID                         *ie.IE
	TraceInformation               *ie.IE
	APNDNN                         *ie.IE
	CreateMAR                      []*ie.IE
	PFCPSEReqFlags                 *ie.IE
	CreateBridgeInfoForTSC         *ie.IE
	CreateSRR                      []*ie.IE
	ProvideATSSSControlInformation *ie.IE
	RecoveryTimeStamp              *ie.IE
	IEs                            []*ie.IE
}

// NewSessionEstablishmentRequest creates a new SessionEstablishmentRequest.
func NewSessionEstablishmentRequest(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionEstablishmentRequest {
	m := &SessionEstablishmentRequest{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionEstablishmentRequest, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.FSEID:
			m.CPFSEID = i
		case ie.CreatePDR:
			m.CreatePDR = append(m.CreatePDR, i)
		case ie.CreateFAR:
			m.CreateFAR = append(m.CreateFAR, i)
		case ie.CreateURR:
			m.CreateURR = append(m.CreateURR, i)
		case ie.CreateQER:
			m.CreateQER = append(m.CreateQER, i)
		case ie.CreateBAR:
			m.CreateBAR = i
		case ie.CreateTrafficEndpoint:
			m.CreateTrafficEndpoint = append(m.CreateTrafficEndpoint, i)
		case ie.PDNType:
			m.PDNType = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.UserID:
			m.UserID = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.APNDNN:
			m.APNDNN = i
		case ie.CreateMAR:
			m.CreateMAR = append(m.CreateMAR, i)
		case ie.PFCPSEReqFlags:
			m.PFCPSEReqFlags = i
		case ie.CreateBridgeInfoForTSC:
			m.CreateBridgeInfoForTSC = i
		case ie.CreateSRR:
			m.CreateSRR = append(m.CreateSRR, i)
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.RecoveryTimeStamp:
			m.RecoveryTimeStamp = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionEstablishmentRequest.
func (m *SessionEstablishmentRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionEstablishmentRequest) MarshalTo(b []byte) error {
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
	if i := m.CPFSEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreatePDR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateFAR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateURR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateQER {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateBAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateTrafficEndpoint {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PDNType; i != nil {
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
	if i := m.UserPlaneInactivityTimer; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UserID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.TraceInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.APNDNN; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateMAR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PFCPSEReqFlags; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateBridgeInfoForTSC; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreateSRR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.ProvideATSSSControlInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.RecoveryTimeStamp; i != nil {
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

// ParseSessionEstablishmentRequest decodes a given byte sequence as a SessionEstablishmentRequest.
func ParseSessionEstablishmentRequest(b []byte) (*SessionEstablishmentRequest, error) {
	m := &SessionEstablishmentRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionEstablishmentRequest.
func (m *SessionEstablishmentRequest) UnmarshalBinary(b []byte) error {
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
		case ie.FSEID:
			m.CPFSEID = i
		case ie.CreatePDR:
			m.CreatePDR = append(m.CreatePDR, i)
		case ie.CreateFAR:
			m.CreateFAR = append(m.CreateFAR, i)
		case ie.CreateURR:
			m.CreateURR = append(m.CreateURR, i)
		case ie.CreateQER:
			m.CreateQER = append(m.CreateQER, i)
		case ie.CreateBAR:
			m.CreateBAR = i
		case ie.CreateTrafficEndpoint:
			m.CreateTrafficEndpoint = append(m.CreateTrafficEndpoint, i)
		case ie.PDNType:
			m.PDNType = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.UserID:
			m.UserID = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.APNDNN:
			m.APNDNN = i
		case ie.CreateMAR:
			m.CreateMAR = append(m.CreateMAR, i)
		case ie.PFCPSEReqFlags:
			m.PFCPSEReqFlags = i
		case ie.CreateBridgeInfoForTSC:
			m.CreateBridgeInfoForTSC = i
		case ie.CreateSRR:
			m.CreateSRR = append(m.CreateSRR, i)
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.RecoveryTimeStamp:
			m.RecoveryTimeStamp = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionEstablishmentRequest) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CPFSEID; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreatePDR {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateFAR {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateURR {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateQER {
		l += i.MarshalLen()
	}
	if i := m.CreateBAR; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateTrafficEndpoint {
		l += i.MarshalLen()
	}
	if i := m.PDNType; i != nil {
		l += i.MarshalLen()
	}
	if i := m.FQCSID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UserPlaneInactivityTimer; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UserID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.TraceInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.APNDNN; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateMAR {
		l += i.MarshalLen()
	}
	if i := m.PFCPSEReqFlags; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateBridgeInfoForTSC; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateSRR {
		l += i.MarshalLen()
	}
	if i := m.ProvideATSSSControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RecoveryTimeStamp; i != nil {
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
func (m *SessionEstablishmentRequest) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionEstablishmentRequest) MessageTypeName() string {
	return "Session Establishment Request"
}

// SEID returns the SEID in uint64.
func (m *SessionEstablishmentRequest) SEID() uint64 {
	return m.Header.seid()
}
