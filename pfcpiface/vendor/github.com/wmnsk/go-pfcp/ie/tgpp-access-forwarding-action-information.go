// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewTGPPAccessForwardingActionInformation creates a new TGPPAccessForwardingActionInformation IE.
func NewTGPPAccessForwardingActionInformation(ies ...*IE) *IE {
	return newGroupedIE(TGPPAccessForwardingActionInformation, 0, ies...)
}

// TGPPAccessForwardingActionInformation returns the IEs above TGPPAccessForwardingActionInformation if the type of IE matches.
func (i *IE) TGPPAccessForwardingActionInformation() ([]*IE, error) {
	switch i.Type {
	case TGPPAccessForwardingActionInformation:
		return ParseMultiIEs(i.Payload)
	case CreateMAR:
		ies, err := i.CreateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TGPPAccessForwardingActionInformation {
				return x.TGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	case UpdateMAR:
		ies, err := i.UpdateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == TGPPAccessForwardingActionInformation {
				return x.TGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
