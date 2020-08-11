// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewTimeThreshold creates a new TimeThreshold IE.
func NewTimeThreshold(threshold uint32) *IE {
	return newUint32ValIE(TimeThreshold, threshold)
}

// TimeThreshold returns TimeThreshold in uint32 if the type of IE matches.
func (i *IE) TimeThreshold() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case TimeThreshold:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TimeThreshold {
				return x.TimeThreshold()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TimeThreshold {
				return x.TimeThreshold()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
