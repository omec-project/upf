// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// NodeReportRequest is a NodeReportRequest formed PFCP Header and its IEs above.
type NodeReportRequest struct {
	*Header
	NodeID                      *ie.IE
	NodeReportType              *ie.IE
	UserPlanePathFailureReport  *ie.IE
	UserPlanePathRecoveryReport *ie.IE
	ClockDriftReport            []*ie.IE
	GTPUPathQoSReport           []*ie.IE
	IEs                         []*ie.IE
}

// NewNodeReportRequest creates a new NodeReportRequest.
func NewNodeReportRequest(seq uint32, ies ...*ie.IE) *NodeReportRequest {
	m := &NodeReportRequest{
		Header: NewHeader(
			1, 0, 0, 0,
			MsgTypeNodeReportRequest, 0, seq, 0,
			nil,
		),
	}

	for _, i := range ies {
		switch i.Type {
		case ie.NodeID:
			m.NodeID = i
		case ie.NodeReportType:
			m.NodeReportType = i
		case ie.UserPlanePathFailureReport:
			m.UserPlanePathFailureReport = i
		case ie.UserPlanePathRecoveryReport:
			m.UserPlanePathRecoveryReport = i
		case ie.ClockDriftReport:
			m.ClockDriftReport = append(m.ClockDriftReport, i)
		case ie.GTPUPathQoSReport:
			m.GTPUPathQoSReport = append(m.GTPUPathQoSReport, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	m.SetLength()
	return m
}

// Marshal returns the byte sequence generated from a NodeReportRequest.
func (m *NodeReportRequest) Marshal() ([]byte, error) {
	b := make([]byte, m.MarshalLen())
	if err := m.MarshalTo(b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (m *NodeReportRequest) MarshalTo(b []byte) error {
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
	if i := m.NodeReportType; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UserPlanePathFailureReport; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	if i := m.UserPlanePathRecoveryReport; i != nil {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.ClockDriftReport {
		if err := i.MarshalTo(m.Payload[offset:]); err != nil {
			return err
		}
		offset += i.MarshalLen()
	}
	for _, i := range m.GTPUPathQoSReport {
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

// ParseNodeReportRequest decodes a given byte sequence as a NodeReportRequest.
func ParseNodeReportRequest(b []byte) (*NodeReportRequest, error) {
	m := &NodeReportRequest{}
	if err := m.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalBinary decodes a given byte sequence as a NodeReportRequest.
func (m *NodeReportRequest) UnmarshalBinary(b []byte) error {
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
		case ie.NodeReportType:
			m.NodeReportType = i
		case ie.UserPlanePathFailureReport:
			m.UserPlanePathFailureReport = i
		case ie.UserPlanePathRecoveryReport:
			m.UserPlanePathRecoveryReport = i
		case ie.ClockDriftReport:
			m.ClockDriftReport = append(m.ClockDriftReport, i)
		case ie.GTPUPathQoSReport:
			m.GTPUPathQoSReport = append(m.GTPUPathQoSReport, i)
		default:
			m.IEs = append(m.IEs, i)
		}
	}

	return nil
}

// MarshalLen returns the serial length of Data.
func (m *NodeReportRequest) MarshalLen() int {
	l := m.Header.MarshalLen() - len(m.Header.Payload)

	if i := m.NodeID; i != nil {
		l += i.MarshalLen()
	}
	if i := m.NodeReportType; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UserPlanePathFailureReport; i != nil {
		l += i.MarshalLen()
	}
	if i := m.UserPlanePathRecoveryReport; i != nil {
		l += i.MarshalLen()
	}
	for _, i := range m.ClockDriftReport {
		l += i.MarshalLen()
	}
	for _, i := range m.GTPUPathQoSReport {
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
func (m *NodeReportRequest) SetLength() {
	m.Header.Length = uint16(m.MarshalLen() - 4)
}

// MessageTypeName returns the name of protocol.
func (m *NodeReportRequest) MessageTypeName() string {
	return "Node Report Request"
}

// SEID returns the SEID in uint64.
func (m *NodeReportRequest) SEID() uint64 {
	return m.Header.seid()
}
