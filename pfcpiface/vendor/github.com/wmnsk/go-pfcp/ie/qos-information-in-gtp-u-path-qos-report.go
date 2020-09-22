// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewQoSInformationInGTPUPathQoSReport creates a new QoSInformationInGTPUPathQoSReport IE.
func NewQoSInformationInGTPUPathQoSReport(ies ...*IE) *IE {
	return newGroupedIE(QoSInformationInGTPUPathQoSReport, 0, ies...)
}

// QoSInformationInGTPUPathQoSReport returns the IEs above QoSInformationInGTPUPathQoSReport if the type of IE matches.
func (i *IE) QoSInformationInGTPUPathQoSReport() ([]*IE, error) {
	switch i.Type {
	case QoSInformationInGTPUPathQoSReport:
		return ParseMultiIEs(i.Payload)
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == QoSInformationInGTPUPathQoSReport {
				return x.QoSInformationInGTPUPathQoSReport()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
