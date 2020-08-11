// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewTrafficEndpointID creates a new TrafficEndpointID IE.
func NewTrafficEndpointID(id uint8) *IE {
	return newUint8ValIE(TrafficEndpointID, id)
}

// TrafficEndpointID returns TrafficEndpointID in uint8 if the type of IE matches.
func (i *IE) TrafficEndpointID() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case TrafficEndpointID:
		return i.Payload[0], nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case CreatedTrafficEndpoint:
		ies, err := i.CreatedTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	case RemoveTrafficEndpoint:
		ies, err := i.RemoveTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TrafficEndpointID {
				return x.TrafficEndpointID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
