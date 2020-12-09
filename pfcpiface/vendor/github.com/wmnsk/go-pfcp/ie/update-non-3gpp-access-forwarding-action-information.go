// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateNonTGPPAccessForwardingActionInformation creates a new UpdateNonTGPPAccessForwardingActionInformation IE.
func NewUpdateNonTGPPAccessForwardingActionInformation(ies ...*IE) *IE {
	return newGroupedIE(UpdateNonTGPPAccessForwardingActionInformation, 0, ies...)
}

// UpdateNonTGPPAccessForwardingActionInformation returns the IEs above UpdateNonTGPPAccessForwardingActionInformation if the type of IE matches.
func (i *IE) UpdateNonTGPPAccessForwardingActionInformation() ([]*IE, error) {
	switch i.Type {
	case UpdateNonTGPPAccessForwardingActionInformation:
		return ParseMultiIEs(i.Payload)
	case UpdateMAR:
		ies, err := i.UpdateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UpdateNonTGPPAccessForwardingActionInformation {
				return x.UpdateNonTGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
