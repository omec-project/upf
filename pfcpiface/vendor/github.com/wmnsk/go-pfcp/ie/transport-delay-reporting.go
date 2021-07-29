// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewTransportDelayReporting creates a new TransportDelayReporting IE.
func NewTransportDelayReporting(ies ...*IE) *IE {
	return newGroupedIE(TransportDelayReporting, 0, ies...)
}

// TransportDelayReporting returns the IEs above TransportDelayReporting if the type of IE matches.
func (i *IE) TransportDelayReporting() ([]*IE, error) {
	switch i.Type {
	case TransportDelayReporting:
		return ParseMultiIEs(i.Payload)
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TransportDelayReporting {
				return x.TransportDelayReporting()
			}
		}
		return nil, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TransportDelayReporting {
				return x.TransportDelayReporting()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
