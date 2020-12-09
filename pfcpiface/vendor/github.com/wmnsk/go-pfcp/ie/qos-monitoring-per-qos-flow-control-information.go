// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewQoSMonitoringPerQoSFlowControlInformation creates a new QoSMonitoringPerQoSFlowControlInformation IE.
func NewQoSMonitoringPerQoSFlowControlInformation(ies ...*IE) *IE {
	return newGroupedIE(QoSMonitoringPerQoSFlowControlInformation, 0, ies...)
}

// QoSMonitoringPerQoSFlowControlInformation returns the IEs above QoSMonitoringPerQoSFlowControlInformation if the type of IE matches.
func (i *IE) QoSMonitoringPerQoSFlowControlInformation() ([]*IE, error) {
	switch i.Type {
	case QoSMonitoringPerQoSFlowControlInformation:
		return ParseMultiIEs(i.Payload)
	case CreateSRR:
		ies, err := i.CreateSRR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == QoSMonitoringPerQoSFlowControlInformation {
				return x.QoSMonitoringPerQoSFlowControlInformation()
			}
		}
		return nil, ErrIENotFound
	case UpdateSRR:
		ies, err := i.UpdateSRR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == QoSMonitoringPerQoSFlowControlInformation {
				return x.QoSMonitoringPerQoSFlowControlInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
