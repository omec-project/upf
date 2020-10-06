// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionModificationResponse is a SessionModificationResponse formed PFCP Header and its IEs above.
//
// TODO: add Packet Rate Status Report IE.
//
// TODO: rename CreatedBridgeInfoForTSC => TSCManagementInformation
type SessionModificationResponse struct {
	*Header
	Cause                             *ie.IE
	OffendingIE                       *ie.IE
	CreatedPDR                        []*ie.IE
	LoadControlInformation            *ie.IE
	OverloadControlInformation        *ie.IE
	UsageReport                       []*ie.IE
	FailedRuleID                      *ie.IE
	AdditionalUsageReportsInformation *ie.IE
	CreatedUpdatedTrafficEndpoint     []*ie.IE
	CreatedBridgeInfoForTSC           *ie.IE
	ATSSSControlParameters            *ie.IE
	UpdatedPDR                        []*ie.IE
	IEs                               []*ie.IE
}

// NewSessionModificationResponse creates a new SessionModificationResponse.
func NewSessionModificationResponse(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionModificationResponse {
	m := &SessionModificationResponse{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionModificationResponse, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.Cause:
			m.Cause = i
		case ie.OffendingIE:
			m.OffendingIE = i
		case ie.CreatedPDR:
			m.CreatedPDR = append(m.CreatedPDR, i)
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.UsageReportWithinSessionModificationResponse:
			m.UsageReport = append(m.UsageReport, i)
		case ie.FailedRuleID:
			m.FailedRuleID = i
		case ie.AdditionalUsageReportsInformation:
			m.AdditionalUsageReportsInformation = i
		case ie.CreatedTrafficEndpoint:
			m.CreatedUpdatedTrafficEndpoint = append(m.CreatedUpdatedTrafficEndpoint, i)
		case ie.CreatedBridgeInfoForTSC:
			m.CreatedBridgeInfoForTSC = i
		case ie.ATSSSControlParameters:
			m.ATSSSControlParameters = i
		case ie.UpdatedPDR:
			m.UpdatedPDR = append(m.UpdatedPDR, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionModificationResponse.
func (m *SessionModificationResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionModificationResponse) MarshalTo(b []byte) error {
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
	for _, i := range m.UsageReport {
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
	if i := m.AdditionalUsageReportsInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.CreatedUpdatedTrafficEndpoint {
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
	for _, i := range m.UpdatedPDR {
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

// ParseSessionModificationResponse decodes a given byte sequence as a SessionModificationResponse.
func ParseSessionModificationResponse(b []byte) (*SessionModificationResponse, error) {
	m := &SessionModificationResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionModificationResponse.
func (m *SessionModificationResponse) UnmarshalBinary(b []byte) error {
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
		case ie.CreatedPDR:
			m.CreatedPDR = append(m.CreatedPDR, i)
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.UsageReportWithinSessionModificationResponse:
			m.UsageReport = append(m.UsageReport, i)
		case ie.FailedRuleID:
			m.FailedRuleID = i
		case ie.AdditionalUsageReportsInformation:
			m.AdditionalUsageReportsInformation = i
		case ie.CreatedTrafficEndpoint:
			m.CreatedUpdatedTrafficEndpoint = append(m.CreatedUpdatedTrafficEndpoint, i)
		case ie.CreatedBridgeInfoForTSC:
			m.CreatedBridgeInfoForTSC = i
		case ie.ATSSSControlParameters:
			m.ATSSSControlParameters = i
		case ie.UpdatedPDR:
			m.UpdatedPDR = append(m.UpdatedPDR, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionModificationResponse) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.Cause; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OffendingIE; i != nil {
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
	for _, i := range m.UsageReport {
		l += i.MarshalLen()
	}
	if i := m.FailedRuleID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.AdditionalUsageReportsInformation; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.CreatedUpdatedTrafficEndpoint {
		l += i.MarshalLen()
	}
	if i := m.CreatedBridgeInfoForTSC; i != nil {
		l += i.MarshalLen()
	}
	if i := m.ATSSSControlParameters; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdatedPDR {
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
func (m *SessionModificationResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionModificationResponse) MessageTypeName() string {
	return "Session Modification Response"
}

// SEID returns the SEID in uint64.
func (m *SessionModificationResponse) SEID() uint64 {
	return m.Header.seid()
}
