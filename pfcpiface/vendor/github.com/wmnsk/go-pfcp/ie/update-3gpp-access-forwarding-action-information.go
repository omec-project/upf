// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateTGPPAccessForwardingActionInformation creates a new UpdateTGPPAccessForwardingActionInformation IE.
func NewUpdateTGPPAccessForwardingActionInformation(ies ...*IE) *IE {
	return newGroupedIE(UpdateTGPPAccessForwardingActionInformation, 0, ies...)
}

// UpdateTGPPAccessForwardingActionInformation returns the IEs above UpdateTGPPAccessForwardingActionInformation if the type of IE matches.
func (i *IE) UpdateTGPPAccessForwardingActionInformation() ([]*IE, error) {
	switch i.Type {
	case UpdateTGPPAccessForwardingActionInformation:
		return ParseMultiIEs(i.Payload)
	case UpdateMAR:
		ies, err := i.UpdateMAR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == UpdateTGPPAccessForwardingActionInformation {
				return x.UpdateTGPPAccessForwardingActionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
