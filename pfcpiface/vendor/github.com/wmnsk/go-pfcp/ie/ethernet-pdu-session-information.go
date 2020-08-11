// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewEthernetPDUSessionInformation creates a new EthernetPDUSessionInformation IE.
func NewEthernetPDUSessionInformation(info uint8) *IE {
	return newUint8ValIE(EthernetPDUSessionInformation, info)
}

// EthernetPDUSessionInformation returns EthernetPDUSessionInformation in []byte if the type of IE matches.
func (i *IE) EthernetPDUSessionInformation() ([]byte, error) {
	switch i.Type {
	case EthernetPDUSessionInformation:
		return i.Payload, nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PDI {
				return x.EthernetPDUSessionInformation()
			}
		}
		return nil, ErrIENotFound
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetPDUSessionInformation {
				return x.EthernetPDUSessionInformation()
			}
		}
		return nil, ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == EthernetPDUSessionInformation {
				return x.EthernetPDUSessionInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasETHI reports whether an IE has ETHI bit.
func (i *IE) HasETHI() bool {
	if i.Type != EthernetPDUSessionInformation {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}
