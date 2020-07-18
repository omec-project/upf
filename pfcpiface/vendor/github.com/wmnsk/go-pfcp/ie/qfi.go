// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewQFI creates a new QFI IE.
func NewQFI(qfi uint8) *IE {
	return newUint8ValIE(QFI, qfi)
}

// QFI returns QFI in uint8 if the type of IE matches.
func (i *IE) QFI() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case QFI:
		return i.Payload[0], nil
	case DownlinkDataServiceInformation:
		if len(i.Payload) < 2 {
			return 0, io.ErrUnexpectedEOF
		}

		return i.Payload[2], nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	case QoSMonitoringReport:
		ies, err := i.QoSMonitoringReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QFI {
				return x.QFI()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
