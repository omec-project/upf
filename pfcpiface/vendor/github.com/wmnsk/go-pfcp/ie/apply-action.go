// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewApplyAction creates a new ApplyAction IE.
func NewApplyAction(flag uint8) *IE {
	return newUint8ValIE(ApplyAction, flag)
}

// ApplyAction returns ApplyAction in uint8 if the type of IE matches.
func (i *IE) ApplyAction() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case ApplyAction:
		return i.Payload[0], nil
	case CreateFAR:
		ies, err := i.CreateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ApplyAction {
				return x.ApplyAction()
			}
		}
		return 0, ErrIENotFound
	case UpdateFAR:
		ies, err := i.UpdateFAR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == ApplyAction {
				return x.ApplyAction()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasDROP reports whether an IE has DROP bit.
func (i *IE) HasDROP() bool {
	v, err := i.ApplyAction()
	if err != nil {
		return false
	}

	return has1stBit(v)
}

// HasFORW reports whether an IE has FORW bit.
func (i *IE) HasFORW() bool {
	v, err := i.ApplyAction()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasBUFF reports whether an IE has BUFF bit.
func (i *IE) HasBUFF() bool {
	v, err := i.ApplyAction()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasNOCP reports whether an IE has NOCP bit.
func (i *IE) HasNOCP() bool {
	v, err := i.ApplyAction()
	if err != nil {
		return false
	}

	return has4thBit(v)
}

// HasDUPL reports whether an IE has DUPL bit.
func (i *IE) HasDUPL() bool {
	v, err := i.ApplyAction()
	if err != nil {
		return false
	}

	return has5thBit(v)
}
