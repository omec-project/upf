// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"time"
)

// BaseTimeIntervalType definitions.
const (
	BTITCTP uint8 = 0
	BTITDTP uint8 = 1
)

// NewTimeQuotaMechanism creates a new TimeQuotaMechanism IE.
func NewTimeQuotaMechanism(btit uint8, bti time.Duration) *IE {
	b := make([]byte, 5)
	b[0] = btit & 0x03
	binary.BigEndian.PutUint32(b[1:5], uint32(bti.Seconds()))

	return New(TimeQuotaMechanism, b)
}

// TimeQuotaMechanism returns TimeQuotaMechanism in []byte if the type of IE matches.
func (i *IE) TimeQuotaMechanism() ([]byte, error) {
	switch i.Type {
	case TimeQuotaMechanism:
		return i.Payload, nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TimeQuotaMechanism {
				return x.TimeQuotaMechanism()
			}
		}
		return nil, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TimeQuotaMechanism {
				return x.TimeQuotaMechanism()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
