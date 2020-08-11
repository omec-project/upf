// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewWeight creates a new Weight IE.
func NewWeight(weight uint8) *IE {
	return newUint8ValIE(Weight, weight)
}

// Weight returns Weight in uint8 if the type of IE matches.
func (i *IE) Weight() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case Weight:
		return i.Payload[0], nil
	case CreateMAR:
		ies, err := i.CreateMAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			switch x.Type {
			case TGPPAccessForwardingActionInformation, NonTGPPAccessForwardingActionInformation:
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	case UpdateMAR:
		ies, err := i.UpdateMAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			switch x.Type {
			case TGPPAccessForwardingActionInformation, NonTGPPAccessForwardingActionInformation:
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	case TGPPAccessForwardingActionInformation:
		ies, err := i.TGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Weight {
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	case NonTGPPAccessForwardingActionInformation:
		ies, err := i.NonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Weight {
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	case UpdateTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Weight {
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	case UpdateNonTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateNonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Weight {
				return x.Weight()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
