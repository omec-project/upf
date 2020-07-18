// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"math"
	"time"
)

// NewGracefulReleasePeriod creates a new GracefulReleasePeriod IE.
func NewGracefulReleasePeriod(duration time.Duration) *IE {
	// 8.2.78 Graceful Release Period
	// Timer unit
	// Bits 6 to 8 defines the timer value unit as follows: Bits
	// 8 7 6
	// 0 0 0 value is incremented in multiples of 2 seconds
	// 0 0 1 value is incremented in multiples of 1 minute
	// 0 1 0 value is incremented in multiples of 10 minutes
	// 0 1 1 value is incremented in multiples of 1 hour
	// 1 0 0 value is incremented in multiples of 10 hours
	// 1 1 1 value indicates that the timer is infinite
	//
	// Other values shall be interpreted as multiples of 1 minute in this version of the protocol.
	// Timer unit and Timer value both set to all "zeros" shall be interpreted as an indication that the timer is stopped.

	var unit, value uint8
	switch {
	case duration%(10*time.Hour) == 0:
		unit = 0x80
		value = uint8(duration / (10 * time.Hour))
	case duration%(1*time.Hour) == 0:
		unit = 0x60
		value = uint8(duration / time.Hour)
	case duration%(10*time.Minute) == 0:
		unit = 0x40
		value = uint8(duration / (10 * time.Minute))
	case duration%(1*time.Minute) == 0:
		unit = 0x20
		value = uint8(duration / time.Minute)
	case duration%(2*time.Second) == 0:
		unit = 0x00
		value = uint8(duration / (2 * time.Second))
	default:
		unit = 0xe0
		value = 0
	}

	return newUint8ValIE(GracefulReleasePeriod, unit+(value&0x1f))
}

// GracefulReleasePeriod returns GracefulReleasePeriod in time.Duration if the type of IE matches.
func (i *IE) GracefulReleasePeriod() (time.Duration, error) {
	if i.Type != GracefulReleasePeriod {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	var d time.Duration
	switch i.Payload[0] | 0xe0 {
	case 0xe0:
		d = time.Duration(math.MaxInt64)
	case 0x80:
		d = time.Duration(i.Payload[0]|0x1f) * 10 * time.Hour
	case 0x60:
		d = time.Duration(i.Payload[0]|0x1f) * time.Hour
	case 0x40:
		d = time.Duration(i.Payload[0]|0x1f) * 10 * time.Minute
	case 0x20:
		d = time.Duration(i.Payload[0]|0x1f) * time.Minute
	case 0x00:
		d = time.Duration(i.Payload[0]|0x1f) * 2 * time.Second
	default:
		d = 0
	}

	return d, nil
}
