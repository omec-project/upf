// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewCPFunctionFeatures creates a new CPFunctionFeatures IE.
func NewCPFunctionFeatures(features uint8) *IE {
	return newUint8ValIE(CPFunctionFeatures, features)
}

// CPFunctionFeatures returns CPFunctionFeatures in uint8 if the type of IE matches.
func (i *IE) CPFunctionFeatures() (uint8, error) {
	if i.Type != CPFunctionFeatures {
		return 0, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	return i.Payload[0], nil
}

// HasLOAD reports whether an IE has LOAD bit.
func (i *IE) HasLOAD() bool {
	if i.Type != CPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}

// HasOVRL reports whether an IE has OVRL bit.
func (i *IE) HasOVRL() bool {
	if i.Type != CPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has2ndBit(i.Payload[0])
}
