// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewATSSSLLInformation creates a new ATSSSLLInformation IE.
func NewATSSSLLInformation(lli uint8) *IE {
	return newUint8ValIE(ATSSSLLInformation, lli&0x01)
}

// ATSSSLLInformation returns ATSSSLLInformation in uint8 if the type of IE matches.
func (i *IE) ATSSSLLInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case ATSSSLLInformation:
		return i.Payload[0], nil
	case ATSSSControlParameters:
		ies, err := i.ATSSSControlParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ATSSSLLParameters {
				return x.ATSSSLLInformation()
			}
		}
		return 0, ErrIENotFound
	case ATSSSLLParameters:
		ies, err := i.ATSSSLLParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ATSSSLLInformation {
				return x.ATSSSLLInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
