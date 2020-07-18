// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
	"time"
)

// NewEventTimeStamp creates a new EventTimeStamp IE.
func NewEventTimeStamp(ts time.Time) *IE {
	u64sec := uint64(ts.Sub(time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC))) / 1000000000
	return newUint32ValIE(EventTimeStamp, uint32(u64sec))
}

// EventTimeStamp returns EventTimeStamp in time.Time if the type of IE matches.
func (i *IE) EventTimeStamp() (time.Time, error) {
	if len(i.Payload) < 4 {
		return time.Time{}, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case EventTimeStamp:
		return time.Unix(int64(binary.BigEndian.Uint32(i.Payload[0:4])-2208988800), 0), nil
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return time.Time{}, err
		}
		for _, x := range ies {
			if x.Type == EventTimeStamp {
				return x.EventTimeStamp()
			}
		}
		return time.Time{}, ErrIENotFound
	case GTPUPathQoSReport:
		ies, err := i.GTPUPathQoSReport()
		if err != nil {
			return time.Time{}, err
		}
		for _, x := range ies {
			if x.Type == EventTimeStamp {
				return x.EventTimeStamp()
			}
		}
		return time.Time{}, ErrIENotFound
	case QoSMonitoringReport:
		ies, err := i.QoSMonitoringReport()
		if err != nil {
			return time.Time{}, err
		}
		for _, x := range ies {
			if x.Type == EventTimeStamp {
				return x.EventTimeStamp()
			}
		}
		return time.Time{}, ErrIENotFound
	case ClockDriftReport:
		ies, err := i.ClockDriftReport()
		if err != nil {
			return time.Time{}, err
		}

		for _, x := range ies {
			if x.Type == EventTimeStamp {
				return x.EventTimeStamp()
			}
		}
		return time.Time{}, ErrIENotFound
	default:
		return time.Time{}, &InvalidTypeError{Type: i.Type}
	}

}
