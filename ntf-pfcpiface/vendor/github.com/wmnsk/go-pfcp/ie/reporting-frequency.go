// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewReportingFrequency creates a new ReportingFrequency IE.
func NewReportingFrequency(sesrl, perio, evett int) *IE {
	return newUint8ValIE(ReportingFrequency, uint8((sesrl<<2)|(perio<<1)|(evett)))
}

// ReportingFrequency returns ReportingFrequency in uint8 if the type of IE matches.
func (i *IE) ReportingFrequency() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case ReportingFrequency:
		return i.Payload[0], nil
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ReportingFrequency {
				return x.ReportingFrequency()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}

// HasSESRL reports whether an IE has SESRL bit.
func (i *IE) HasSESRL() bool {
	v, err := i.ReportingFrequency()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasEVETT reports whether an IE has EVETT bit.
func (i *IE) HasEVETT() bool {
	v, err := i.ReportingFrequency()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
