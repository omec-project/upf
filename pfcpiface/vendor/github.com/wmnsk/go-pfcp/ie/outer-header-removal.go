// Copyright 2019-2021 go-pfcp authors. All rights reserved.
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
	// The size of the OuterHeaderRemoval IE was one octet in 3GPP TS 29.244 up to V15.3.0,
	// but it has been changed to two octets since V15.4.0.
	// For backward compatibility, one octet is also accepted.
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

	// If the size of the payload is less than two octets because the original was formatted before
	// 3GPP TS 29.244 V15.3.0, 0 is returned as GTPUExternsionHeaderDeletion.
	if len(v) < 2 {
		return 0, io.ErrUnexpectedEOF
	}

	return v[1], nil
}
