// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewMinimumWaitTime creates a new MinimumWaitTime IE.
//
// the period should be within the range of uint32, otherwise it overflows.
func NewMinimumWaitTime(period time.Duration) *IE {
	return newUint32ValIE(MinimumWaitTime, uint32(period.Seconds()))
}

// MinimumWaitTime returns MinimumWaitTime in time.Duration if the type of IE matches.
func (i *IE) MinimumWaitTime() (time.Duration, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case MinimumWaitTime:
		return time.Duration(binary.BigEndian.Uint32(i.Payload[0:4])) * time.Second, nil
	case QoSMonitoringPerQoSFlowControlInformation:
		ies, err := i.QoSMonitoringPerQoSFlowControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == MinimumWaitTime {
				return x.MinimumWaitTime()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
