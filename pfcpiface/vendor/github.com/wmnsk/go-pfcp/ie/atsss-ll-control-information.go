// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewATSSSLLControlInformation creates a new ATSSSLLControlInformation IE.
func NewATSSSLLControlInformation(lli uint8) *IE {
	return newUint8ValIE(ATSSSLLControlInformation, lli&0x01)
}

// ATSSSLLControlInformation returns ATSSSLLControlInformation in uint8 if the type of IE matches.
func (i *IE) ATSSSLLControlInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case ATSSSLLControlInformation:
		return i.Payload[0], nil
	case ATSSSControlParameters:
		ies, err := i.ATSSSControlParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ATSSSLLControlInformation {
				return x.ATSSSLLControlInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasLLI reports whether an IE has LLI bit.
func (i *IE) HasLLI() bool {
	switch i.Type {
	case ATSSSLLControlInformation, ATSSSLLInformation:
		if len(i.Payload) < 1 {
			return false
		}

		return has1stBit(i.Payload[0])
	default:
		return false
	}
}
