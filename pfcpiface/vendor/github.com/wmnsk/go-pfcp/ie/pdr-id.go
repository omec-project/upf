// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewPDRID creates a new PDRID IE.
func NewPDRID(id uint16) *IE {
	return newUint16ValIE(PDRID, id)
}

// PDRID returns PDRID in uint16 if the type of IE matches.
func (i *IE) PDRID() (uint16, error) {
	switch i.Type {
	case PDRID:
		if len(i.Payload) < 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return binary.BigEndian.Uint16(i.Payload[0:2]), nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case RemovePDR:
		ies, err := i.RemovePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case CreatedPDR:
		ies, err := i.CreatedPDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case ApplicationDetectionInformation:
		ies, err := i.ApplicationDetectionInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ApplicationDetectionInformation {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case DownlinkDataReport:
		ies, err := i.DownlinkDataReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	case UpdatedPDR:
		ies, err := i.UpdatedPDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDRID {
				return x.PDRID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
