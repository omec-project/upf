// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// SessionModificationRequest is a SessionModificationRequest formed PFCP Header and its IEs above.
type SessionModificationRequest struct {
	*Header
	CPFSEID                         *ie.IE
	RemovePDR                       *ie.IE
	RemoveFAR                       *ie.IE
	RemoveURR                       *ie.IE
	RemoveQER                       *ie.IE
	RemoveBAR                       *ie.IE
	RemoveTrafficEndpoint           *ie.IE
	CreatePDR                       *ie.IE
	CreateFAR                       *ie.IE
	CreateURR                       *ie.IE
	CreateQER                       *ie.IE
	CreateBAR                       *ie.IE
	CreateTrafficEndpoint           *ie.IE
	UpdatePDR                       *ie.IE
	UpdateFAR                       *ie.IE
	UpdateURR                       *ie.IE
	UpdateQER                       *ie.IE
	UpdateBAR                       *ie.IE
	UpdateTrafficEndpoint           *ie.IE
	PFCPSMReqFlags                  *ie.IE
	QueryURR                        *ie.IE
	FQCSID                          *ie.IE
	UserPlaneInactivityTimer        *ie.IE
	QueryURRReference               *ie.IE
	TraceInformation                *ie.IE
	RemoveMAR                       *ie.IE
	UpdateMAR                       *ie.IE
	CreateMAR                       *ie.IE
	NodeID                          *ie.IE
	PortManagementInformationForTSC *ie.IE
	RemoveSRR                       *ie.IE
	CreateSRR                       *ie.IE
	UpdateSRR                       *ie.IE
	ProvideATSSSControlInformation  *ie.IE
	EthernetContextInformation      *ie.IE
	AccessAvailabilityInformation   *ie.IE
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
			m.RemovePDR = i
		case ie.RemoveFAR:
			m.RemoveFAR = i
		case ie.RemoveURR:
			m.RemoveURR = i
		case ie.RemoveQER:
			m.RemoveQER = i
		case ie.RemoveBAR:
			m.RemoveBAR = i
		case ie.RemoveTrafficEndpoint:
			m.RemoveTrafficEndpoint = i
		case ie.CreatePDR:
			m.CreatePDR = i
		case ie.CreateFAR:
			m.CreateFAR = i
		case ie.CreateURR:
			m.CreateURR = i
		case ie.CreateQER:
			m.CreateQER = i
		case ie.CreateBAR:
			m.CreateBAR = i
		case ie.CreateTrafficEndpoint:
			m.CreateTrafficEndpoint = i
		case ie.UpdatePDR:
			m.UpdatePDR = i
		case ie.UpdateFAR:
			m.UpdateFAR = i
		case ie.UpdateURR:
			m.UpdateURR = i
		case ie.UpdateQER:
			m.UpdateQER = i
		case ie.UpdateBARWithinSessionModificationRequest:
			m.UpdateBAR = i
		case ie.UpdateTrafficEndpoint:
			m.UpdateTrafficEndpoint = i
		case ie.PFCPSMReqFlags:
			m.PFCPSMReqFlags = i
		case ie.QueryURR:
			m.QueryURR = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.QueryURRReference:
			m.QueryURRReference = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.RemoveMAR:
			m.RemoveMAR = i
		case ie.UpdateMAR:
			m.UpdateMAR = i
		case ie.CreateMAR:
			m.CreateMAR = i
		case ie.NodeID:
			m.NodeID = i
		case ie.PortManagementInformationForTSCWithinSessionModificationRequest:
			m.PortManagementInformationForTSC = i
		case ie.RemoveSRR:
			m.RemoveSRR = i
		case ie.CreateSRR:
			m.CreateSRR = i
		case ie.UpdateSRR:
			m.UpdateSRR = i
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.EthernetContextInformation:
			m.EthernetContextInformation = i
		case ie.AccessAvailabilityInformation:
			m.AccessAvailabilityInformation = i
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
	if i := m.RemovePDR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.RemoveFAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.RemoveURR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.RemoveQER; i != nil {
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
	if i := m.RemoveTrafficEndpoint; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreatePDR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateFAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateURR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateQER; i != nil {
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
	if i := m.CreateTrafficEndpoint; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdatePDR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdateFAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdateURR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdateQER; i != nil {
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
	if i := m.UpdateTrafficEndpoint; i != nil {
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
	if i := m.QueryURR; i != nil {
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
	if i := m.RemoveMAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdateMAR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateMAR; i != nil {
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
	if i := m.RemoveSRR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CreateSRR; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UpdateSRR; i != nil {
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
	if i := m.AccessAvailabilityInformation; i != nil {
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
			m.RemovePDR = i
		case ie.RemoveFAR:
			m.RemoveFAR = i
		case ie.RemoveURR:
			m.RemoveURR = i
		case ie.RemoveQER:
			m.RemoveQER = i
		case ie.RemoveBAR:
			m.RemoveBAR = i
		case ie.RemoveTrafficEndpoint:
			m.RemoveTrafficEndpoint = i
		case ie.CreatePDR:
			m.CreatePDR = i
		case ie.CreateFAR:
			m.CreateFAR = i
		case ie.CreateURR:
			m.CreateURR = i
		case ie.CreateQER:
			m.CreateQER = i
		case ie.CreateBAR:
			m.CreateBAR = i
		case ie.CreateTrafficEndpoint:
			m.CreateTrafficEndpoint = i
		case ie.UpdatePDR:
			m.UpdatePDR = i
		case ie.UpdateFAR:
			m.UpdateFAR = i
		case ie.UpdateURR:
			m.UpdateURR = i
		case ie.UpdateQER:
			m.UpdateQER = i
		case ie.UpdateBARWithinSessionModificationRequest:
			m.UpdateBAR = i
		case ie.UpdateTrafficEndpoint:
			m.UpdateTrafficEndpoint = i
		case ie.PFCPSMReqFlags:
			m.PFCPSMReqFlags = i
		case ie.QueryURR:
			m.QueryURR = i
		case ie.FQCSID:
			m.FQCSID = i
		case ie.UserPlaneInactivityTimer:
			m.UserPlaneInactivityTimer = i
		case ie.QueryURRReference:
			m.QueryURRReference = i
		case ie.TraceInformation:
			m.TraceInformation = i
		case ie.RemoveMAR:
			m.RemoveMAR = i
		case ie.UpdateMAR:
			m.UpdateMAR = i
		case ie.CreateMAR:
			m.CreateMAR = i
		case ie.NodeID:
			m.NodeID = i
		case ie.PortManagementInformationForTSCWithinSessionModificationRequest:
			m.PortManagementInformationForTSC = i
		case ie.RemoveSRR:
			m.RemoveSRR = i
		case ie.CreateSRR:
			m.CreateSRR = i
		case ie.UpdateSRR:
			m.UpdateSRR = i
		case ie.ProvideATSSSControlInformation:
			m.ProvideATSSSControlInformation = i
		case ie.EthernetContextInformation:
			m.EthernetContextInformation = i
		case ie.AccessAvailabilityInformation:
			m.AccessAvailabilityInformation = i
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
	if i := m.RemovePDR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveFAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveURR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveQER; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveBAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveTrafficEndpoint; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreatePDR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateFAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateURR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateQER; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateBAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateTrafficEndpoint; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdatePDR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateFAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateURR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateQER; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateBAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateTrafficEndpoint; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PFCPSMReqFlags; i != nil {
		l += i.MarshalLen()
	}
	if i := m.QueryURR; i != nil {
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
	if i := m.RemoveMAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateMAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateMAR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.PortManagementInformationForTSC; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RemoveSRR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CreateSRR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UpdateSRR; i != nil {
		l += i.MarshalLen()
	}
	if i := m.ProvideATSSSControlInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.EthernetContextInformation; i != nil {
		l += i.MarshalLen()
	}
	if i := m.AccessAvailabilityInformation; i != nil {
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
