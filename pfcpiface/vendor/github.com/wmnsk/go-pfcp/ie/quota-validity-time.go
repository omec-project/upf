// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewQuotaValidityTime creates a new QuotaValidityTime IE.
func NewQuotaValidityTime(ts time.Time) *IE {
	u64sec := uint64(ts.Sub(time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC))) / 1000000000
	return newUint32ValIE(QuotaValidityTime, uint32(u64sec))
}

// QuotaValidityTime returns QuotaValidityTime in time.Time if the type of IE matches.
func (i *IE) QuotaValidityTime() (time.Time, error) {
	if len(i.Payload) < 4 {
		return time.Time{}, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case QuotaValidityTime:
		return time.Unix(int64(binary.BigEndian.Uint32(i.Payload[0:4])-2208988800), 0), nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return time.Time{}, err
		}
		for _, x := range ies {
			if x.Type == QuotaValidityTime {
				return x.QuotaValidityTime()
			}
		}
		return time.Time{}, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return time.Time{}, err
		}
		for _, x := range ies {
			if x.Type == QuotaValidityTime {
				return x.QuotaValidityTime()
			}
		}
		return time.Time{}, ErrIENotFound
	default:
		return time.Time{}, &InvalidTypeError{Type: i.Type}
	}
}
