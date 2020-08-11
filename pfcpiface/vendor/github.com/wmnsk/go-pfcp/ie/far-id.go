// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewFARID creates a new FARID IE.
func NewFARID(id uint32) *IE {
	return newUint32ValIE(FARID, id)
}

// FARID returns FARID in uint32 if the type of IE matches.
func (i *IE) FARID() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case FARID:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case RemoveFAR:
		ies, err := i.RemoveFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			switch x.Type {
			case TGPPAccessForwardingActionInformation, NonTGPPAccessForwardingActionInformation:
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case CreateMAR:
		ies, err := i.CreateMAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			switch x.Type {
			case TGPPAccessForwardingActionInformation, NonTGPPAccessForwardingActionInformation:
				return x.FARID()
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
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case TGPPAccessForwardingActionInformation:
		ies, err := i.TGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case NonTGPPAccessForwardingActionInformation:
		ies, err := i.NonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	case UpdateNonTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateNonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == FARID {
				return x.FARID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
