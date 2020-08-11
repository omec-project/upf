// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewQoSReportTrigger creates a new QoSReportTrigger IE.
func NewQoSReportTrigger(ire, thr, per int) *IE {
	return newUint8ValIE(QoSReportTrigger, uint8((ire<<2)|(thr<<1)|(per)))
}

// QoSReportTrigger returns QoSReportTrigger in uint8 if the type of IE matches.
func (i *IE) QoSReportTrigger() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case QoSReportTrigger:
		return i.Payload[0], nil
	case GTPUPathQoSControlInformation:
		ies, err := i.GTPUPathQoSControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QoSReportTrigger {
				return x.QoSReportTrigger()
			}
		}
		return 0, ErrIENotFound
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QoSReportTrigger {
				return x.QoSReportTrigger()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}

// HasIRE reports whether an IE has IRE bit.
func (i *IE) HasIRE() bool {
	v, err := i.QoSReportTrigger()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasTHR reports whether an IE has THR bit.
func (i *IE) HasTHR() bool {
	v, err := i.QoSReportTrigger()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasPER reports whether an IE has PER bit.
func (i *IE) HasPER() bool {
	v, err := i.QoSReportTrigger()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
