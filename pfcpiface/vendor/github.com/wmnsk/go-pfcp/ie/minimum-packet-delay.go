// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewMinimumPacketDelay creates a new MinimumPacketDelay IE.
//
// the delay should be within the range of uint32, otherwise it overflows.
func NewMinimumPacketDelay(delay time.Duration) *IE {
	return newUint32ValIE(MinimumPacketDelay, uint32(delay.Milliseconds()))
}

// MinimumPacketDelay returns MinimumPacketDelay in time.Duration if the type of IE matches.
func (i *IE) MinimumPacketDelay() (time.Duration, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MinimumPacketDelay:
		return time.Duration(binary.BigEndian.Uint32(i.Payload[0:4])) * time.Millisecond, nil
	case GTPUPathQoSControlInformation:
		ies, err := i.GTPUPathQoSControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MinimumPacketDelay {
				return x.MinimumPacketDelay()
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
				return x.MinimumPacketDelay()
			}
		}
		return 0, ErrIENotFound
	case QoSInformationInGTPUPathQoSReport:
		ies, err := i.QoSInformationInGTPUPathQoSReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MinimumPacketDelay {
				return x.MinimumPacketDelay()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
