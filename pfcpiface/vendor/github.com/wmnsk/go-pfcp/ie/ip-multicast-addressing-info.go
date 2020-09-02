// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewIPMulticastAddressingInfo creates a new IPMulticastAddressingInfo IE.
func NewIPMulticastAddressingInfo(ies ...*IE) *IE {
	return newGroupedIE(IPMulticastAddressingInfo, 0, ies...)
}

// IPMulticastAddressingInfo returns the IEs above IPMulticastAddressingInfo if the type of IE matches.
func (i *IE) IPMulticastAddressingInfo() ([]*IE, error) {
	switch i.Type {
	case IPMulticastAddressingInfo:
		return ParseMultiIEs(i.Payload)
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddressingInfo {
				return x.IPMulticastAddressingInfo()
			}
		}
		return nil, ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == IPMulticastAddressingInfo {
				return x.IPMulticastAddressingInfo()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
