// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewEthernetPDUSessionInformation creates a new EthernetPDUSessionInformation IE.
func NewEthernetPDUSessionInformation(info uint8) *IE {
	return newUint8ValIE(EthernetPDUSessionInformation, info)
}

// EthernetPDUSessionInformation returns EthernetPDUSessionInformation in uint8 if the type of IE matches.
func (i *IE) EthernetPDUSessionInformation() (uint8, error) {
	switch i.Type {
	case EthernetPDUSessionInformation:
		return i.Payload[0], nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.EthernetPDUSessionInformation()
			}
		}
		return 0, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetPDUSessionInformation {
				return x.EthernetPDUSessionInformation()
			}
		}
		return 0, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == EthernetPDUSessionInformation {
				return x.EthernetPDUSessionInformation()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// HasETHI reports whether an IE has ETHI bit.
func (i *IE) HasETHI() bool {
	v, err := i.EthernetPDUSessionInformation()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
