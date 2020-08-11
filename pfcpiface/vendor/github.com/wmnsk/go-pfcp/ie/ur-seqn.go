// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewURSEQN creates a new URSEQN IE.
func NewURSEQN(seq uint32) *IE {
	return newUint32ValIE(URSEQN, seq)
}

// URSEQN returns URSEQN in uint32 if the type of IE matches.
func (i *IE) URSEQN() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case URSEQN:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case UsageReportWithinSessionModificationResponse,
		UsageReportWithinSessionDeletionResponse,
		UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == URSEQN {
				return x.URSEQN()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
