// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSubsequentTimeThreshold creates a new SubsequentTimeThreshold IE.
func NewSubsequentTimeThreshold(threshold uint32) *IE {
	return newUint32ValIE(SubsequentTimeThreshold, threshold)
}

// SubsequentTimeThreshold returns SubsequentTimeThreshold in uint32 if the type of IE matches.
func (i *IE) SubsequentTimeThreshold() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case SubsequentTimeThreshold:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeThreshold {
				return x.SubsequentTimeThreshold()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeThreshold {
				return x.SubsequentTimeThreshold()
			}
		}
		return 0, ErrIENotFound
	case AdditionalMonitoringTime:
		ies, err := i.AdditionalMonitoringTime()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeThreshold {
				return x.SubsequentTimeThreshold()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
