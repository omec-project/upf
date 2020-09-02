// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewEthernetPacketFilter creates a new EthernetPacketFilter IE.
func NewEthernetPacketFilter(ies ...*IE) *IE {
	return newGroupedIE(EthernetPacketFilter, 0, ies...)
}

// EthernetPacketFilter returns the IEs above EthernetPacketFilter if the type of IE matches.
func (i *IE) EthernetPacketFilter() ([]*IE, error) {
	switch i.Type {
	case EthernetPacketFilter:
		return ParseMultiIEs(i.Payload)
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.EthernetPacketFilter()
			}
		}
		return nil, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetPacketFilter {
				return x.EthernetPacketFilter()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
