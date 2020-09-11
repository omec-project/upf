// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// FlowDirection definitions.
const (
	FlowDirectionUnspecified   uint8 = 0
	FlowDirectionDownlink      uint8 = 1
	FlowDirectionUplink        uint8 = 2
	FlowDirectionBidirectional uint8 = 3
)

// NewFlowInformation creates a new FlowInformation IE.
func NewFlowInformation(dir uint8, desc string) *IE {
	d := []byte(desc)
	l := len(d)

	i := New(FlowInformation, make([]byte, 3+l))
	i.Payload[0] = dir
	binary.BigEndian.PutUint16(i.Payload[1:3], uint16(l))
	copy(i.Payload[3:], d)

	return i
}

// FlowInformation returns FlowInformation in []byte if the type of IE matches.
func (i *IE) FlowInformation() ([]byte, error) {
	switch i.Type {
	case FlowInformation:
		return i.Payload, nil
	case ApplicationDetectionInformation:
		ies, err := i.ApplicationDetectionInformation()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == FlowInformation {
				return x.FlowInformation()
			}
		}
		return nil, ErrIENotFound
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ApplicationDetectionInformation {
				return x.FlowInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// FlowDirection returns FlowDirection in uint8 if the type of IE matches.
func (i *IE) FlowDirection() (uint8, error) {
	switch i.Type {
	case FlowInformation:
		if len(i.Payload) < 1 {
			return 0, io.ErrUnexpectedEOF
		}
		return i.Payload[0] & 0x07, nil
	case ApplicationDetectionInformation:
		ies, err := i.ApplicationDetectionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FlowInformation {
				return x.FlowDirection()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// FlowDescription returns FlowDescription in string if the type of IE matches.
func (i *IE) FlowDescription() (string, error) {
	switch i.Type {
	case FlowInformation:
		l := binary.BigEndian.Uint16(i.Payload[1:3])
		if len(i.Payload) < int(l) {
			return "", io.ErrUnexpectedEOF
		}

		return string(i.Payload[3 : 3+l]), nil
	case ApplicationDetectionInformation:
		ies, err := i.ApplicationDetectionInformation()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == FlowInformation {
				return x.FlowDescription()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
