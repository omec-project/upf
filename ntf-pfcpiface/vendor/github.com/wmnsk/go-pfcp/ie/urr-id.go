// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewURRID creates a new URRID IE.
func NewURRID(id uint32) *IE {
	return newUint32ValIE(URRID, id)
}

// URRID returns URRID in uint32 if the type of IE matches.
func (i *IE) URRID() (uint32, error) {
	switch i.Type {
	case URRID:
		if len(i.Payload) < 4 {
			return 0, io.ErrUnexpectedEOF
		}

		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case RemoveURR:
		ies, err := i.RemoveURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
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
				return x.URRID()
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
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case QueryURR:
		ies, err := i.QueryURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case TGPPAccessForwardingActionInformation:
		ies, err := i.TGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case NonTGPPAccessForwardingActionInformation:
		ies, err := i.NonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case UpdateTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	case UpdateNonTGPPAccessForwardingActionInformation:
		ies, err := i.UpdateNonTGPPAccessForwardingActionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URRID {
				return x.URRID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// IsAllocatedByCPFunction reports whether URRID is allocated by CP Function.
func (i *IE) IsAllocatedByCPFunction() bool {
	if i.Type != URRID {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return (i.Payload[0]>>7)&0x01 == 1
}

// IsAllocatedByUPFunction reports whether URRID is allocated by UP Function.
func (i *IE) IsAllocatedByUPFunction() bool {
	if i.Type != URRID {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return (i.Payload[0]>>7)&0x01 != 1
}
