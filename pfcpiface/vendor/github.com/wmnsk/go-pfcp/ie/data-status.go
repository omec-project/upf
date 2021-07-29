// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewDataStatus creates a new DataStatus IE.
func NewDataStatus(flag uint8) *IE {
	return newUint8ValIE(DataStatus, flag)
}

// DataStatus returns DataStatus in uint8 if the type of IE matches.
func (i *IE) DataStatus() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case DataStatus:
		return i.Payload[0], nil
	case DownlinkDataReport:
		ies, err := i.DownlinkDataReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == DataStatus {
				return x.DataStatus()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
