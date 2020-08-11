// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewOCIFlags creates a new OCIFlags IE.
func NewOCIFlags(flags uint8) *IE {
	return newUint8ValIE(OCIFlags, flags)
}

// OCIFlags returns OCIFlags in uint8 if the type of IE matches.
func (i *IE) OCIFlags() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case OCIFlags:
		return i.Payload[0], nil
	case OverloadControlInformation:
		ies, err := i.OverloadControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == OCIFlags {
				return x.OCIFlags()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
