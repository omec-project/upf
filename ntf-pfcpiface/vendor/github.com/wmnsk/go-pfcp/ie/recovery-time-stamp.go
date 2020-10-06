// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewRecoveryTimeStamp creates a new RecoveryTimeStamp IE.
func NewRecoveryTimeStamp(ts time.Time) *IE {
	u64sec := uint64(ts.Sub(time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC))) / 1000000000
	return newUint32ValIE(RecoveryTimeStamp, uint32(u64sec))
}

// RecoveryTimeStamp returns RecoveryTimeStamp in time.Time if the type of IE matches.
func (i *IE) RecoveryTimeStamp() (time.Time, error) {
	if i.Type != RecoveryTimeStamp {
		return time.Time{}, &InvalidTypeError{Type: i.Type}
	}

	if len(i.Payload) < 4 {
		return time.Time{}, io.ErrUnexpectedEOF
	}
	return time.Unix(int64(binary.BigEndian.Uint32(i.Payload[0:4])-2208988800), 0), nil
}
