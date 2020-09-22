// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewNonTGPPAccessForwardingActionInformation creates a new NonTGPPAccessForwardingActionInformation IE.
func NewNonTGPPAccessForwardingActionInformation(ies ...*IE) *IE {
	return newGroupedIE(NonTGPPAccessForwardingActionInformation, 0, ies...)
}

// NonTGPPAccessForwardingActionInformation returns the IEs above NonTGPPAccessForwardingActionInformation if the type of IE matches.
func (i *IE) NonTGPPAccessForwardingActionInformation() ([]*IE, error) {
	switch i.Type {
	case NonTGPPAccessForwardingActionInformation:
		return ParseMultiIEs(i.Payload)
	case CreateMAR:
		ies, err := i.CreateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == NonTGPPAccessForwardingActionInformation {
				return x.NonTGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	case UpdateMAR:
		ies, err := i.UpdateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == NonTGPPAccessForwardingActionInformation {
				return x.NonTGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
