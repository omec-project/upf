// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// AssociationUpdateResponse is a AssociationUpdateResponse formed PFCP Header and its IEs above.
type AssociationUpdateResponse struct {
	*Header
	NodeID             *ie.IE
	Cause              *ie.IE
	UPFunctionFeatures *ie.IE
	CPFunctionFeatures *ie.IE
	IEs                []*ie.IE
}

// NewAssociationUpdateResponse creates a new AssociationUpdateResponse.
func NewAssociationUpdateResponse(seq uint32, ies ...*ie.IE) *AssociationUpdateResponse {
	m := &AssociationUpdateResponse{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeAssociationUpdateResponse, 0, seq, 0,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.Cause:
			m.Cause = i
		case ie.UPFunctionFeatures:
			m.UPFunctionFeatures = i
		case ie.CPFunctionFeatures:
			m.CPFunctionFeatures = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a AssociationUpdateResponse.
func (m *AssociationUpdateResponse) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *AssociationUpdateResponse) MarshalTo(b []byte) error {
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

// ParseAssociationUpdateResponse decodes a given byte sequence as a AssociationUpdateResponse.
func ParseAssociationUpdateResponse(b []byte) (*AssociationUpdateResponse, error) {
	m := &AssociationUpdateResponse{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a AssociationUpdateResponse.
func (m *AssociationUpdateResponse) UnmarshalBinary(b []byte) error {
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
		case ie.UPFunctionFeatures:
			m.UPFunctionFeatures = i
		case ie.CPFunctionFeatures:
			m.CPFunctionFeatures = i
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *AssociationUpdateResponse) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.Cause; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UPFunctionFeatures; i != nil {
		l += i.MarshalLen()
	}
	if i := m.CPFunctionFeatures; i != nil {
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
func (m *AssociationUpdateResponse) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *AssociationUpdateResponse) MessageTypeName() string {
	return "Association Update Request"
}

// SEID returns the SEID in uint64.
func (m *AssociationUpdateResponse) SEID() uint64 {
	return m.Header.seid()
}
