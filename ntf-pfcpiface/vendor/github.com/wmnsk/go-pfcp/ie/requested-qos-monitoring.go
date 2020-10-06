// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRequestedQoSMonitoring creates a new RequestedQoSMonitoring IE.
func NewRequestedQoSMonitoring(rp, ul, dl int) *IE {
	return newUint8ValIE(RequestedQoSMonitoring, uint8((rp<<2)|(ul<<1)|(dl)))
}

// RequestedQoSMonitoring returns RequestedQoSMonitoring in uint8 if the type of IE matches.
func (i *IE) RequestedQoSMonitoring() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RequestedQoSMonitoring:
		return i.Payload[0], nil
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == RequestedQoSMonitoring {
				return x.RequestedQoSMonitoring()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}

// HasRP reports whether an IE has RP bit.
func (i *IE) HasRP() bool {
	v, err := i.RequestedQoSMonitoring()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasUL reports whether an IE has UL bit.
func (i *IE) HasUL() bool {
	v, err := i.RequestedQoSMonitoring()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasDL reports whether an IE has DL bit.
func (i *IE) HasDL() bool {
	v, err := i.RequestedQoSMonitoring()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
