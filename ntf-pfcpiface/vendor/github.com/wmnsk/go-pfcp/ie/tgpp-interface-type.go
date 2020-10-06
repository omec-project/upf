// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// TGPPInterfaceType definitons.
const (
	TGPPInterfaceTypeS1U                      uint8 = 0
	TGPPInterfaceTypeS5S8U                    uint8 = 1
	TGPPInterfaceTypeS4U                      uint8 = 2
	TGPPInterfaceTypeS11U                     uint8 = 3
	TGPPInterfaceTypeS12U                     uint8 = 4
	TGPPInterfaceTypeGnGpU                    uint8 = 5
	TGPPInterfaceTypeS2aU                     uint8 = 6
	TGPPInterfaceTypeS2bU                     uint8 = 7
	TGPPInterfaceTypeENBDL                    uint8 = 8
	TGPPInterfaceTypeENBUL                    uint8 = 9
	TGPPInterfaceTypeSGWUPFDL                 uint8 = 10
	TGPPInterfaceTypeN33GPPAccess             uint8 = 11
	TGPPInterfaceTypeN3TrustedNon3GPPAccess   uint8 = 12
	TGPPInterfaceTypeN3UnTrustedNon3GPPAccess uint8 = 13
	TGPPInterfaceTypeN3ForDataForwarding      uint8 = 14
	TGPPInterfaceTypeN9                       uint8 = 15
	TGPPInterfaceTypeSGi                      uint8 = 16
	TGPPInterfaceTypeN6                       uint8 = 17
	TGPPInterfaceTypeN19                      uint8 = 18
	TGPPInterfaceTypeS8U                      uint8 = 19
	TGPPInterfaceTypeGpU                      uint8 = 20
)

// NewTGPPInterfaceType creates a new TGPPInterfaceType IE.
func NewTGPPInterfaceType(intf uint8) *IE {
	return newUint8ValIE(TGPPInterfaceType, intf)
}

// TGPPInterfaceType returns TGPPInterfaceType in uint8 if the type of IE matches.
func (i *IE) TGPPInterfaceType() (uint8, error) {
	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case TGPPInterfaceType:
		return i.Payload[0] & 0x3f, nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TGPPInterfaceType {
				return x.TGPPInterfaceType()
			}
		}
		return 0, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == TGPPInterfaceType {
				return x.TGPPInterfaceType()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
