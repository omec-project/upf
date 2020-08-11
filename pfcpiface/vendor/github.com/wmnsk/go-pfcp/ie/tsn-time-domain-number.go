// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewTSNTimeDomainNumber creates a new TSNTimeDomainNumber IE.
func NewTSNTimeDomainNumber(num uint8) *IE {
	return newUint8ValIE(TSNTimeDomainNumber, num)
}

// TSNTimeDomainNumber returns TSNTimeDomainNumber in uint8 if the type of IE matches.
func (i *IE) TSNTimeDomainNumber() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case TSNTimeDomainNumber:
		return i.Payload[0], nil
	case ClockDriftControlInformation:
		ies, err := i.ClockDriftControlInformation()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == TSNTimeDomainNumber {
				return x.TSNTimeDomainNumber()
			}
		}
		return 0, ErrIENotFound
	case ClockDriftReport:
		ies, err := i.ClockDriftReport()
		if err != nil {
			return 0, err
		}

		for _, x := range ies {
			if x.Type == TSNTimeDomainNumber {
				return x.TSNTimeDomainNumber()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
