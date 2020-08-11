// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewUsageInformation creates a new UsageInformation IE.
func NewUsageInformation(bef, aft, uae, ube int) *IE {
	return newUint8ValIE(UsageInformation, uint8((ube<<3)|(uae<<2)|(aft<<1)|(bef)))
}

// UsageInformation returns UsageInformation in uint8 if the type of IE matches.
func (i *IE) UsageInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case UsageInformation:
		return i.Payload[0], nil
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == UsageInformation {
				return x.UsageInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasUBE reports whether and IE has UBE bit.
func (i *IE) HasUBE() bool {
	v, err := i.UsageInformation()
	if err != nil {
		return false
	}

	return has4thBit(v)
}

// HasUAE reports whether and IE has UAE bit.
func (i *IE) HasUAE() bool {
	v, err := i.UsageInformation()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasAFT reports whether and IE has AFT bit.
func (i *IE) HasAFT() bool {
	v, err := i.UsageInformation()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasBEF reports whether and IE has BEF bit.
func (i *IE) HasBEF() bool {
	v, err := i.UsageInformation()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
