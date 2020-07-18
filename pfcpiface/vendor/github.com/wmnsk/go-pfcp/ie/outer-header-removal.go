// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewOuterHeaderRemoval creates a new OuterHeaderRemoval IE.
func NewOuterHeaderRemoval(desc, ext uint8) *IE {
	return newUint16ValIE(OuterHeaderRemoval, uint16(desc)<<8|uint16(ext))
}

// OuterHeaderRemoval returns OuterHeaderRemoval in []byte if the type of IE matches.
func (i *IE) OuterHeaderRemoval() ([]byte, error) {
	if len(i.Payload) < 1 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case OuterHeaderRemoval:
		return i.Payload, nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == OuterHeaderRemoval {
				return x.OuterHeaderRemoval()
			}
		}
		return nil, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == OuterHeaderRemoval {
				return x.OuterHeaderRemoval()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// OuterHeaderRemovalDescription returns OuterHeaderRemovalDescription in uint8 if the type of IE matches.
func (i *IE) OuterHeaderRemovalDescription() (uint8, error) {
	v, err := i.OuterHeaderRemoval()
	if err != nil {
		return 0, err
	}

	return v[0], nil
}

// GTPUExternsionHeaderDeletion returns GTPUExternsionHeaderDeletion in uint8 if the type of IE matches.
func (i *IE) GTPUExternsionHeaderDeletion() (uint8, error) {
	v, err := i.OuterHeaderRemoval()
	if err != nil {
		return 0, err
	}

	return v[1], nil
}
