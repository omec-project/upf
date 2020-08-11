// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"time"
)

// NewSubsequentTimeQuota creates a new SubsequentTimeQuota IE.
//
// the period should be within the range of uint32, otherwise it overflows.
func NewSubsequentTimeQuota(period time.Duration) *IE {
	return newUint32ValIE(SubsequentTimeQuota, uint32(period.Seconds()))
}

// SubsequentTimeQuota returns SubsequentTimeQuota in time.Duration if the type of IE matches.
func (i *IE) SubsequentTimeQuota() (time.Duration, error) {
	switch i.Type {
	case SubsequentTimeQuota:
		return time.Duration(binary.BigEndian.Uint32(i.Payload[0:4])) * time.Second, nil
	case CreateURR:
		ies, err := i.CreateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeQuota {
				return x.SubsequentTimeQuota()
			}
		}
		return 0, ErrIENotFound
	case UpdateURR:
		ies, err := i.UpdateURR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeQuota {
				return x.SubsequentTimeQuota()
			}
		}
		return 0, ErrIENotFound
	case AdditionalMonitoringTime:
		ies, err := i.AdditionalMonitoringTime()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SubsequentTimeQuota {
				return x.SubsequentTimeQuota()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
