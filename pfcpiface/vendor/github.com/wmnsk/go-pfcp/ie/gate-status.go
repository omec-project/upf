// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// GateStatus definitions.
const (
	GateStatusOpen   uint8 = 0
	GateStatusClosed uint8 = 1
)

// NewGateStatus creates a new GateStatus IE.
func NewGateStatus(ul, dl uint8) *IE {
	return newUint8ValIE(GateStatus, (ul<<2)|dl)
}

// GateStatus returns GateStatus in uint8 if the type of IE matches.
func (i *IE) GateStatus() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case GateStatus:
		return i.Payload[0], nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == GateStatus {
				return x.GateStatus()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == GateStatus {
				return x.GateStatus()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// GateStatusUL returns GateStatusUL in uint8 if the type of IE matches.
func (i *IE) GateStatusUL() (uint8, error) {
	v, err := i.GateStatus()
	if err != nil {
		return 0, err
	}

	return (v >> 2) & 0xff, nil
}

// GateStatusDL returns GateStatusDL in uint8 if the type of IE matches.
func (i *IE) GateStatusDL() (uint8, error) {
	v, err := i.GateStatus()
	if err != nil {
		return 0, err
	}

	return v & 0xff, nil
}
