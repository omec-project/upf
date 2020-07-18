// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewSuggestedBufferingPacketsCount creates a new SuggestedBufferingPacketsCount IE.
func NewSuggestedBufferingPacketsCount(count uint8) *IE {
	return newUint8ValIE(SuggestedBufferingPacketsCount, count)
}

// SuggestedBufferingPacketsCount returns SuggestedBufferingPacketsCount in uint8 if the type of IE matches.
func (i *IE) SuggestedBufferingPacketsCount() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case SuggestedBufferingPacketsCount:
		return i.Payload[0], nil
	case CreateBAR:
		ies, err := i.CreateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SuggestedBufferingPacketsCount {
				return x.SuggestedBufferingPacketsCount()
			}
		}
		return 0, ErrIENotFound
	case UpdateBARWithinSessionReportResponse,
		UpdateBARWithinSessionModificationRequest:
		ies, err := i.CreateBAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == SuggestedBufferingPacketsCount {
				return x.SuggestedBufferingPacketsCount()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
