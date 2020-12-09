// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"

	"github.com/wmnsk/go-pfcp/internal/utils"
)

// NewTraceInformation creates a new TraceInformation IE.
func NewTraceInformation(mcc, mnc, id string, events []byte, depth uint8, interfaces []byte, ip net.IP) *IE {
	fields := NewTraceInformationFields(mcc, mnc, id, events, depth, interfaces, ip)

	b, err := fields.Marshal()
	if err != nil {
		return nil
	}

	return New(TraceInformation, b)
}

// TraceInformation returns TraceInformation in structured format if the type of IE matches.
func (i *IE) TraceInformation() (*TraceInformationFields, error) {
	switch i.Type {
	case TraceInformation:
		fields, err := ParseTraceInformationFields(i.Payload)
		if err != nil {
			return nil, err
		}

		return fields, nil
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// TraceInformationFields represents a fields contained in TraceInformation IE.
type TraceInformationFields struct {
	MCC, MNC, TraceID                      string
	TriggeringEventsLength                 uint8
	TriggeringEvents                       []byte
	SessionTraceDepth                      uint8
	ListOfInterfacesLength                 uint8
	ListOfInterfaces                       []byte
	IPAddressOfTraceCollectionEntityLength uint8
	IPAddressOfTraceCollectionEntity       net.IP
}

// NewTraceInformationFields creates a new NewTraceInformationFields.
func NewTraceInformationFields(mcc, mnc, id string, events []byte, depth uint8, interfaces []byte, ip net.IP) *TraceInformationFields {
	f := &TraceInformationFields{
		MCC: mcc, MNC: mnc, TraceID: id,
		TriggeringEventsLength: uint8(len(events)),
		TriggeringEvents:       events,
		SessionTraceDepth:      depth,
		ListOfInterfacesLength: uint8(len(interfaces)),
		ListOfInterfaces:       interfaces,
	}

	// IPv4
	v4 := ip.To4()
	if v4 != nil {
		f.IPAddressOfTraceCollectionEntityLength = 4
		f.IPAddressOfTraceCollectionEntity = v4
		return f
	}

	//IPv6
	f.IPAddressOfTraceCollectionEntityLength = 16
	f.IPAddressOfTraceCollectionEntity = ip
	return f
}

// ParseTraceInformationFields parses b into TraceInformationFields.
func ParseTraceInformationFields(b []byte) (*TraceInformationFields, error) {
	f := &TraceInformationFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *TraceInformationFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 6 {
		return io.ErrUnexpectedEOF
	}

	var err error
	f.MCC, f.MNC, err = utils.DecodePLMN(b[0:3])
	if err != nil {
		return err
	}
	f.TraceID = string(b[3:6])
	offset := 6

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	f.TriggeringEventsLength = b[offset]
	offset += 1

	if l < offset+int(f.TriggeringEventsLength) {
		return io.ErrUnexpectedEOF
	}
	copy(f.TriggeringEvents, b[offset:offset+int(f.TriggeringEventsLength)])
	offset += int(f.TriggeringEventsLength)

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	f.SessionTraceDepth = b[offset]
	offset += 1

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	f.ListOfInterfacesLength = b[offset]
	offset += 1

	if l < offset+int(f.ListOfInterfacesLength) {
		return io.ErrUnexpectedEOF
	}
	copy(f.ListOfInterfaces, b[offset:offset+int(f.ListOfInterfacesLength)])
	offset += int(f.ListOfInterfacesLength)

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	f.IPAddressOfTraceCollectionEntityLength = b[offset]
	offset += 1

	if l < offset+int(f.IPAddressOfTraceCollectionEntityLength) {
		return io.ErrUnexpectedEOF
	}
	copy(f.IPAddressOfTraceCollectionEntity, b[offset:offset+int(f.IPAddressOfTraceCollectionEntityLength)])

	return nil
}

// Marshal returns the serialized bytes of TraceInformationFields.
func (f *TraceInformationFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *TraceInformationFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 6 {
		return io.ErrUnexpectedEOF
	}

	plmn, err := utils.EncodePLMN(f.MCC, f.MNC)
	if err != nil {
		return err
	}
	copy(b[0:3], plmn)
	copy(b[3:6], []byte(f.TraceID))
	offset := 6

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	b[offset] = f.TriggeringEventsLength
	offset += 1

	if l < offset+int(f.TriggeringEventsLength) {
		return io.ErrUnexpectedEOF
	}
	copy(b[offset:offset+int(f.TriggeringEventsLength)], f.TriggeringEvents)
	offset += int(f.TriggeringEventsLength)

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	b[offset] = f.SessionTraceDepth
	offset += 1

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	b[offset] = f.ListOfInterfacesLength
	offset += 1

	if l < offset+int(f.ListOfInterfacesLength) {
		return io.ErrUnexpectedEOF
	}
	copy(b[offset:offset+int(f.ListOfInterfacesLength)], f.ListOfInterfaces)
	offset += int(f.ListOfInterfacesLength)

	if l < offset {
		return io.ErrUnexpectedEOF
	}
	b[offset] = f.IPAddressOfTraceCollectionEntityLength
	offset += 1

	if l < offset+int(f.IPAddressOfTraceCollectionEntityLength) {
		return io.ErrUnexpectedEOF
	}
	copy(b[offset:offset+int(f.IPAddressOfTraceCollectionEntityLength)], f.IPAddressOfTraceCollectionEntity)

	return nil
}

// MarshalLen returns field length in integer.
func (f *TraceInformationFields) MarshalLen() int {
	l := 3 + 3 + 1 + 1 + 1 + 1
	l += int(f.TriggeringEventsLength)
	l += int(f.ListOfInterfacesLength)
	l += int(f.IPAddressOfTraceCollectionEntityLength)

	return l
}
