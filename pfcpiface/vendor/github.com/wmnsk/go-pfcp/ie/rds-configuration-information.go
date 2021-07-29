// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewRDSConfigurationInformation creates a new RDSConfigurationInformation IE.
func NewRDSConfigurationInformation(rds uint8) *IE {
	return newUint8ValIE(RDSConfigurationInformation, rds&0x01)
}

// RDSConfigurationInformation returns RDSConfigurationInformation in uint8 if the type of IE matches.
func (i *IE) RDSConfigurationInformation() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case RDSConfigurationInformation:
		return i.Payload[0], nil
	case ProvideRDSConfigurationInformation:
		ies, err := i.ProvideRDSConfigurationInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == RDSConfigurationInformation {
				return x.RDSConfigurationInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasRDS reports whether an IE has RDS bit.
func (i *IE) HasRDS() bool {
	switch i.Type {
	case RDSConfigurationInformation:
		if len(i.Payload) < 1 {
			return false
		}

		return has1stBit(i.Payload[0])
	default:
		return false
	}
}
