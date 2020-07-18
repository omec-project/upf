// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"time"
)

// NewDownlinkDataNotificationDelay creates a new DownlinkDataNotificationDelay IE.
func NewDownlinkDataNotificationDelay(delay time.Duration) *IE {
	// TS29.244 8.2.28 Downlink Data Notification Delay
	// Delay Value in integer multiples of 50 millisecs, or zero
	return newUint8ValIE(DownlinkDataNotificationDelay, uint8(delay/50000000))
}

// DownlinkDataNotificationDelay returns DownlinkDataNotificationDelay in time.Duration if the type of IE matches.
func (i *IE) DownlinkDataNotificationDelay() (time.Duration, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case DownlinkDataNotificationDelay:
		return time.Duration(int64(i.Payload[0]) * 50000000), nil
	case CreateBAR:
		ies, err := i.CreateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DownlinkDataNotificationDelay {
				return x.DownlinkDataNotificationDelay()
			}
		}
		return 0, ErrIENotFound
	case UpdateBARWithinSessionReportResponse,
		UpdateBARWithinSessionModificationRequest:
		ies, err := i.UpdateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DownlinkDataNotificationDelay {
				return x.DownlinkDataNotificationDelay()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
