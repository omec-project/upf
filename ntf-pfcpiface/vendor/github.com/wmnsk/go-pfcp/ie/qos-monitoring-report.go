// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewQoSMonitoringReport creates a new QoSMonitoringReport IE.
func NewQoSMonitoringReport(ies ...*IE) *IE {
	return newGroupedIE(QoSMonitoringReport, 0, ies...)
}

// QoSMonitoringReport returns the IEs above QoSMonitoringReport if the type of IE matches.
func (i *IE) QoSMonitoringReport() ([]*IE, error) {
	switch i.Type {
	case QoSMonitoringReport:
		return ParseMultiIEs(i.Payload)
	case SessionReport:
		ies, err := i.SessionReport()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == QoSMonitoringReport {
				return x.QoSMonitoringReport()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
