// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
)

// NewTransportLevelMarking creates a new TransportLevelMarking IE.
func NewTransportLevelMarking(tos uint16) *IE {
	return newUint16ValIE(TransportLevelMarking, tos)
}

// TransportLevelMarking returns TransportLevelMarking in uint16 if the type of IE matches.
func (i *IE) TransportLevelMarking() (uint16, error) {
	if len(i.Payload) < 2 {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	switch i.Type {
	case TransportLevelMarking:
		return binary.BigEndian.Uint16(i.Payload[0:2]), nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case DuplicatingParameters:
		ies, err := i.DuplicatingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case UpdateDuplicatingParameters:
		ies, err := i.UpdateDuplicatingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case GTPUPathQoSControlInformation:
		ies, err := i.GTPUPathQoSControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QoSInformationInGTPUPathQoSReport {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	case QoSInformationInGTPUPathQoSReport:
		ies, err := i.QoSInformationInGTPUPathQoSReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TransportLevelMarking {
				return x.TransportLevelMarking()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}

}
