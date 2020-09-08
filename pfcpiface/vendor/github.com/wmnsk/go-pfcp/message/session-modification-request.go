// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionModificationRequest is a SessionModificationRequest formed PFCP Header and its IEs above.
//
// TODO: add Query Packet Rate Status IE
//
// TODO: rename PortManagementInformationForTSC => TSCManagementInformation
type SessionModificationRequest struct {
	*Header
	CPFSEID                         *ie.IE
	RemovePDR                       []*ie.IE
	RemoveFAR                       []*ie.IE
	RemoveURR                       []*ie.IE
	RemoveQER                       []*ie.IE
	RemoveBAR                       *ie.IE
	RemoveTrafficEndpoint           []*ie.IE
	CreatePDR                       []*ie.IE
	CreateFAR                       []*ie.IE
	CreateURR                       []*ie.IE
	CreateQER                       []*ie.IE
	CreateBAR                       *ie.IE
	CreateTrafficEndpoint           []*ie.IE
	UpdatePDR                       []*ie.IE
	UpdateFAR                       []*ie.IE
	UpdateURR                       []*ie.IE
	UpdateQER                       []*ie.IE
	UpdateBAR                       *ie.IE
	UpdateTrafficEndpoint           []*ie.IE
	PFCPSMReqFlags                  *ie.IE
	QueryURR                        []*ie.IE
	FQCSID                          *ie.IE
	UserPlaneInactivityTimer        *ie.IE
	QueryURRReference               *ie.IE
	TraceInformation                *ie.IE
	RemoveMAR                       []*ie.IE
	UpdateMAR                       []*ie.IE
	CreateMAR                       []*ie.IE
	NodeID                          *ie.IE
	PortManagementInformationForTSC *ie.IE
	RemoveSRR                       []*ie.IE
	CreateSRR                       []*ie.IE
	UpdateSRR                       []*ie.IE
	ProvideATSSSControlInformation  *ie.IE
	EthernetContextInformation      *ie.IE
	AccessAvailabilityInformation   []*ie.IE
	IEs                             []*ie.IE
}

// NewSessionModificationRequest creates a new SessionModificationRequest.
func NewSessionModificationRequest(mp, fo uint8, seid uint64, seq uint32, pri uint8, ies ...*ie.IE) *SessionModificationRequest {
	m := &SessionModificationRequest{
		Header: NewHeader(
			1, fo, mp, 1,
			MsgTypeSessionModificationRequest, seid, seq, pri,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.FSEID:
			m.CPFSEID = i
		case ie.RemovePDR:
			m.RemovePDR = append(m.RemovePDR, i)
		case ie.RemoveFAR:
			m.RemoveFAR = append(m.RemoveFAR, i)
		case ie.RemoveURR:
			m.RemoveURR = append(m.RemoveURR, i)
		case ie.RemoveQER:
			m.RemoveQER = append(m.RemoveQER, i)
		case ie.RemoveBAR:
			m.RemoveBAR = i
		case ie.RemoveTrafficEndpoint:
			m.RemoveTrafficEndpoint = append(m.RemoveTrafficEndpoint, i)
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
		case ie.UpdatePDR:
			m.UpdatePDR = append(m.UpdatePDR, i)
		case ie.UpdateFAR:
			m.UpdateFAR = append(m.UpdateFAR, i)
		case ie.UpdateURR:
			m.UpdateURR = append(m.UpdateURR, i)
		case ie.UpdateQER:
			m.UpdateQER = append(m.UpdateQER, i)
		case ie.UpdateBARWithinSessionModificationRequest:
			m.UpdateBAR = i
		case ie.UpdateTrafficEndpoint:
			m.UpdateTrafficEndpoint = append(m.UpdateTrafficEndpoint, i)
		case ie.PFCPSMReqFlags:
			m.PFCPSMReqFlags = i
		case ie.QueryURR:
			m.QueryURR = append(m.QueryURR, i)
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.QueryURRReference:
			m.QueryURRReference = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.RemoveMAR:
			m.RemoveMAR = append(m.RemoveMAR, i)
		case ie.UpdateMAR:
			m.UpdateMAR = append(m.UpdateMAR, i)
		case ie.CreateMAR:
			m.CreateMAR = append(m.CreateMAR, i)
		case ie.NodeID:
			m.NodeID = i
		case ie.PortManagementInformationForTSCWithinSessionModificationRequest:
			m.PortManagementInformationForTSC = i
		case ie.RemoveSRR:
			m.RemoveSRR = append(m.RemoveSRR, i)
		case ie.CreateSRR:
			m.CreateSRR = append(m.CreateSRR, i)
		case ie.UpdateSRR:
			m.UpdateSRR = append(m.UpdateSRR, i)
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.EthernetContextInformation:
			m.EthernetContextInformation = i
		case ie.AccessAvailabilityInformation:
			m.AccessAvailabilityInformation = append(m.AccessAvailabilityInformation, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a SessionModificationRequest.
func (m *SessionModificationRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *SessionModificationRequest) MarshalTo(b []byte) error {
	if m.Header.Payload != nil {
		m.Header.Payload = nil
	}
	m.Header.Payload = make([]byte, m.MarshalLen()-m.Header.MarshalLen())

	offset := 0
	if i := m.CPFSEID; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.RemovePDR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.RemoveFAR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.RemoveURR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.RemoveQER {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.RemoveBAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.RemoveTrafficEndpoint {
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
	for _, i := range m.UpdatePDR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UpdateFAR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UpdateURR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UpdateQER {
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
	for _, i := range m.UpdateTrafficEndpoint {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PFCPSMReqFlags; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.QueryURR {
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
	if i := m.QueryURRReference; i != nil {
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
	for _, i := range m.RemoveMAR {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UpdateMAR {
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
	if i := m.NodeID; i != nil {
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
	for _, i := range m.RemoveSRR {
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
	for _, i := range m.UpdateSRR {
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
	if i := m.EthernetContextInformation; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.AccessAvailabilityInformation {
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

// ParseSessionModificationRequest decodes a given byte sequence as a SessionModificationRequest.
func ParseSessionModificationRequest(b []byte) (*SessionModificationRequest, error) {
	m := &SessionModificationRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a SessionModificationRequest.
func (m *SessionModificationRequest) UnmarshalBinary(b []byte) error {
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
		case ie.FSEID:
			m.CPFSEID = i
		case ie.RemovePDR:
			m.RemovePDR = append(m.RemovePDR, i)
		case ie.RemoveFAR:
			m.RemoveFAR = append(m.RemoveFAR, i)
		case ie.RemoveURR:
			m.RemoveURR = append(m.RemoveURR, i)
		case ie.RemoveQER:
			m.RemoveQER = append(m.RemoveQER, i)
		case ie.RemoveBAR:
			m.RemoveBAR = i
		case ie.RemoveTrafficEndpoint:
			m.RemoveTrafficEndpoint = append(m.RemoveTrafficEndpoint, i)
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
		case ie.UpdatePDR:
			m.UpdatePDR = append(m.UpdatePDR, i)
		case ie.UpdateFAR:
			m.UpdateFAR = append(m.UpdateFAR, i)
		case ie.UpdateURR:
			m.UpdateURR = append(m.UpdateURR, i)
		case ie.UpdateQER:
			m.UpdateQER = append(m.UpdateQER, i)
		case ie.UpdateBARWithinSessionModificationRequest:
			m.UpdateBAR = i
		case ie.UpdateTrafficEndpoint:
			m.UpdateTrafficEndpoint = append(m.UpdateTrafficEndpoint, i)
		case ie.PFCPSMReqFlags:
			m.PFCPSMReqFlags = i
		case ie.QueryURR:
			m.QueryURR = append(m.QueryURR, i)
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.QueryURRReference:
			m.QueryURRReference = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.RemoveMAR:
			m.RemoveMAR = append(m.RemoveMAR, i)
		case ie.UpdateMAR:
			m.UpdateMAR = append(m.UpdateMAR, i)
		case ie.CreateMAR:
			m.CreateMAR = append(m.CreateMAR, i)
		case ie.NodeID:
			m.NodeID = i
		case ie.PortManagementInformationForTSCWithinSessionModificationRequest:
			m.PortManagementInformationForTSC = i
		case ie.RemoveSRR:
			m.RemoveSRR = append(m.RemoveSRR, i)
		case ie.CreateSRR:
			m.CreateSRR = append(m.CreateSRR, i)
		case ie.UpdateSRR:
			m.UpdateSRR = append(m.UpdateSRR, i)
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.EthernetContextInformation:
			m.EthernetContextInformation = i
		case ie.AccessAvailabilityInformation:
			m.AccessAvailabilityInformation = append(m.AccessAvailabilityInformation, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *SessionModificationRequest) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.CPFSEID; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.RemovePDR {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveFAR {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveURR {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveQER {
		l += i.MarshalLen()
	}
	if i := m.RemoveBAR; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveTrafficEndpoint {
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
	for _, i := range m.UpdatePDR {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateFAR {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateURR {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateQER {
		l += i.MarshalLen()
	}
	if i := m.UpdateBAR; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateTrafficEndpoint {
		l += i.MarshalLen()
	}
	if i := m.PFCPSMReqFlags; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.QueryURR {
		l += i.MarshalLen()
	}
	if i := m.FQCSID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UserPlaneInactivityTimer; i != nil {
		l += i.MarshalLen()
	}
	if i := m.QueryURRReference; i != nil {
		l += i.MarshalLen()
	}
	if i := m.TraceInformation; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveMAR {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateMAR {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateMAR {
		l += i.MarshalLen()
	}
	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PortManagementInformationForTSC; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.RemoveSRR {
		l += i.MarshalLen()
	}
	for _, i := range m.CreateSRR {
		l += i.MarshalLen()
	}
	for _, i := range m.UpdateSRR {
		l += i.MarshalLen()
	}
	if i := m.ProvideATSSSControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.EthernetContextInformation; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.AccessAvailabilityInformation {
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
func (m *SessionModificationRequest) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *SessionModificationRequest) MessageTypeName() string {
	return "Session Modification Request"
}

// SEID returns the SEID in uint64.
func (m *SessionModificationRequest) SEID() uint64 {
	return m.Header.seid()
}
