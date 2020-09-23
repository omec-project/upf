// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionReportRequest is a SessionReportRequest formed PFCP Header and its IEs above.
//
// TODO: rename PortManagementInformationForTSC => TSCManagementInformation
type SessionReportRequest struct {
	*Header
	ReportType                        *ie.IE
	DownlinkDataReport                *ie.IE
	UsageReport                       []*ie.IE
	ErrorIndicationReport             *ie.IE
	LoadControlInformation            *ie.IE
	OverloadControlInformation        *ie.IE
	AdditionalUsageReportsInformation *ie.IE
	PFCPSRReqFlags                    *ie.IE
	OldCPFSEID                        *ie.IE
	PacketRateStatusReport            *ie.IE
	PortManagementInformationForTSC   *ie.IE
	SessionReport                     []*ie.IE
	IEs                               []*ie.IE
}

// NewSessionReportRequest creates a new SessionReportRequest.
func NewSessionReportRequest(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionReportRequest {
	m := &SessionReportRequest{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionReportRequest, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.ReportType:
			m.ReportType = i
		case ie.DownlinkDataReport:
			m.DownlinkDataReport = i
		case ie.UsageReportWithinSessionReportRequest:
			m.UsageReport = append(m.UsageReport, i)
		case ie.ErrorIndicationReport:
			m.ErrorIndicationReport = i
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.AdditionalUsageReportsInformation:
			m.AdditionalUsageReportsInformation = i
		case ie.PFCPSRReqFlags:
			m.PFCPSRReqFlags = i
		case ie.FSEID:
			m.OldCPFSEID = i
		case ie.PacketRateStatusReport:
			m.PacketRateStatusReport = i
		case ie.PortManagementInformationForTSCWithinSessionReportRequest:
			m.PortManagementInformationForTSC = i
		case ie.SessionReport:
			m.SessionReport = append(m.SessionReport, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionReportRequest.
func (m *SessionReportRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionReportRequest) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
	if i := m.ReportType; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.DownlinkDataReport; i != nil {
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
	if i := m.ErrorIndicationReport; i != nil {
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
	if i := m.AdditionalUsageReportsInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PFCPSRReqFlags; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.OldCPFSEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PacketRateStatusReport; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PortManagementInformationForTSC; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.SessionReport {
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

// ParseSessionReportRequest decodes a given byte sequence as a SessionReportRequest.
func ParseSessionReportRequest(b []byte) (*SessionReportRequest, error) {
	m := &SessionReportRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionReportRequest.
func (m *SessionReportRequest) UnmarshalBinary(b []byte) error {
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
		case ie.ReportType:
			m.ReportType = i
		case ie.DownlinkDataReport:
			m.DownlinkDataReport = i
		case ie.UsageReportWithinSessionReportRequest:
			m.UsageReport = append(m.UsageReport, i)
		case ie.ErrorIndicationReport:
			m.ErrorIndicationReport = i
		case ie.LoadControlInformation:
			m.LoadControlInformation = i
		case ie.OverloadControlInformation:
			m.OverloadControlInformation = i
		case ie.AdditionalUsageReportsInformation:
			m.AdditionalUsageReportsInformation = i
		case ie.PFCPSRReqFlags:
			m.PFCPSRReqFlags = i
		case ie.FSEID:
			m.OldCPFSEID = i
		case ie.PacketRateStatusReport:
			m.PacketRateStatusReport = i
		case ie.PortManagementInformationForTSCWithinSessionReportRequest:
			m.PortManagementInformationForTSC = i
		case ie.SessionReport:
			m.SessionReport = append(m.SessionReport, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionReportRequest) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.ReportType; i != nil {
		l += i.MarshalLen()
	}
	if i := m.DownlinkDataReport; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.UsageReport {
		l += i.MarshalLen()
	}
	if i := m.ErrorIndicationReport; i != nil {
		l += i.MarshalLen()
	}
	if i := m.LoadControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OverloadControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.AdditionalUsageReportsInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PFCPSRReqFlags; i != nil {
		l += i.MarshalLen()
	}
	if i := m.OldCPFSEID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PacketRateStatusReport; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PortManagementInformationForTSC; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.SessionReport {
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
func (m *SessionReportRequest) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionReportRequest) MessageTypeName() string {
	return "Session Report Request"
}

// SEID returns the SEID in uint64.
func (m *SessionReportRequest) SEID() uint64 {
	return m.Header.seid()
}
