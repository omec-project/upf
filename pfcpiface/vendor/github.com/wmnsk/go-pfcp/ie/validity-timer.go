// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewValidityTimer creates a new ValidityTimer IE.
//
// the period should be within the range of uint16, otherwise it overflows.
func NewValidityTimer(period time.Duration) *IE {
	return newUint16ValIE(ValidityTimer, uint16(period.Seconds()))
}

// ValidityTimer returns ValidityTimer in time.Duration if the type of IE matches.
func (i *IE) ValidityTimer() (time.Duration, error) {
	if len(i.Payload) < 2 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case ValidityTimer:
		return time.Duration(binary.BigEndian.Uint16(i.Payload[0:2])) * time.Second, nil
	case UEIPAddressUsageInformation:
		ies, err := i.UEIPAddressUsageInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ValidityTimer {
				return x.ValidityTimer()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
