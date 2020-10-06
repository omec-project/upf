// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// AssociationSetupResponse is a AssociationSetupResponse formed PFCP Header and its IEs above.
type AssociationSetupResponse struct {
	*Header
	NodeID                         *ie.IE
	Cause                          *ie.IE
	RecoveryTimeStamp              *ie.IE
	UPFunctionFeatures             *ie.IE
	CPFunctionFeatures             *ie.IE
	UserPlaneIPResourceInformation []*ie.IE
	AlternativeSMFIPAddress        []*ie.IE
	PFCPASRspFlags                 *ie.IE
	UEIPAddressPoolInformation     []*ie.IE
	GTPUPathQoSControlInformation  []*ie.IE
	ClockDriftControlInformation   []*ie.IE
	UPFInstanceID                  *ie.IE
	IEs                            []*ie.IE
}

// NewAssociationSetupResponse creates a new AssociationSetupResponse.
func NewAssociationSetupResponse(seq uint32, ies ...*ie.IE) *AssociationSetupResponse {
	m := &AssociationSetupResponse{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeAssociationSetupResponse, 0, seq, 0,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.Cause:
			m.Cause = i
		case ie.RecoveryTimeStamp:
			m.RecoveryTimeStamp = i
		case ie.UPFunctionFeatures:
			m.UPFunctionFeatures = i
		case ie.CPFunctionFeatures:
			m.CPFunctionFeatures = i
		case ie.UserPlaneIPResourceInformation:
			m.UserPlaneIPResourceInformation = append(m.UserPlaneIPResourceInformation, i)
		case ie.AlternativeSMFIPAddress:
			m.AlternativeSMFIPAddress = append(m.AlternativeSMFIPAddress, i)
		case ie.PFCPASRspFlags:
			m.PFCPASRspFlags = i
		case ie.UEIPAddressPoolInformation:
			m.UEIPAddressPoolInformation = append(m.UEIPAddressPoolInformation, i)
		case ie.GTPUPathQoSControlInformation:
			m.GTPUPathQoSControlInformation = append(m.GTPUPathQoSControlInformation, i)
		case ie.ClockDriftControlInformation:
			m.ClockDriftControlInformation = append(m.ClockDriftControlInformation, i)
		case ie.NFInstanceID:
			m.UPFInstanceID = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a AssociationSetupResponse.
func (m *AssociationSetupResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *AssociationSetupResponse) MarshalTo(b []byte) error {
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
	if i := m.RecoveryTimeStamp; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UPFunctionFeatures; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.CPFunctionFeatures; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UserPlaneIPResourceInformation {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.AlternativeSMFIPAddress {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.PFCPASRspFlags; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.UEIPAddressPoolInformation {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.GTPUPathQoSControlInformation {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.ClockDriftControlInformation {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UPFInstanceID; i != nil {
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

// ParseAssociationSetupResponse decodes a given byte sequence as a AssociationSetupResponse.
func ParseAssociationSetupResponse(b []byte) (*AssociationSetupResponse, error) {
	m := &AssociationSetupResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a AssociationSetupResponse.
func (m *AssociationSetupResponse) UnmarshalBinary(b []byte) error {
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
		case ie.RecoveryTimeStamp:
			m.RecoveryTimeStamp = i
		case ie.UPFunctionFeatures:
			m.UPFunctionFeatures = i
		case ie.CPFunctionFeatures:
			m.CPFunctionFeatures = i
		case ie.UserPlaneIPResourceInformation:
			m.UserPlaneIPResourceInformation = append(m.UserPlaneIPResourceInformation, i)
		case ie.AlternativeSMFIPAddress:
			m.AlternativeSMFIPAddress = append(m.AlternativeSMFIPAddress, i)
		case ie.PFCPASRspFlags:
			m.PFCPASRspFlags = i
		case ie.UEIPAddressPoolInformation:
			m.UEIPAddressPoolInformation = append(m.UEIPAddressPoolInformation, i)
		case ie.GTPUPathQoSControlInformation:
			m.GTPUPathQoSControlInformation = append(m.GTPUPathQoSControlInformation, i)
		case ie.ClockDriftControlInformation:
			m.ClockDriftControlInformation = append(m.ClockDriftControlInformation, i)
		case ie.NFInstanceID:
			m.UPFInstanceID = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *AssociationSetupResponse) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.Cause; i != nil {
		l += i.MarshalLen()
	}
	if i := m.RecoveryTimeStamp; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UPFunctionFeatures; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CPFunctionFeatures; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.UserPlaneIPResourceInformation {
		l += i.MarshalLen()
	}
	for _, i := range m.AlternativeSMFIPAddress {
		l += i.MarshalLen()
	}
	if i := m.PFCPASRspFlags; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.UEIPAddressPoolInformation {
		l += i.MarshalLen()
	}
	for _, i := range m.GTPUPathQoSControlInformation {
		l += i.MarshalLen()
	}
	for _, i := range m.ClockDriftControlInformation {
		l += i.MarshalLen()
	}
	if i := m.UPFInstanceID; i != nil {
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
func (m *AssociationSetupResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *AssociationSetupResponse) MessageTypeName() string {
	return "Association Setup Response"
}

// SEID returns the SEID in uint64.
func (m *AssociationSetupResponse) SEID() uint64 {
	return m.Header.seid()
}
