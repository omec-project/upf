// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewSubsequentEventQuota creates a new SubsequentEventQuota IE.
func NewSubsequentEventQuota(quota uint32) *IE {
	return newUint32ValIE(SubsequentEventQuota, quota)
}

// SubsequentEventQuota returns SubsequentEventQuota in uint32 if the type of IE matches.
func (i *IE) SubsequentEventQuota() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case SubsequentEventQuota:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentEventQuota {
				return x.SubsequentEventQuota()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentEventQuota {
				return x.SubsequentEventQuota()
			}
		}
		return 0, ErrIENotFound
	case AdditionalMonitoringTime:
		ies, err := i.AdditionalMonitoringTime()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentEventQuota {
				return x.SubsequentEventQuota()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
