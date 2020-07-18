// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUPFunctionFeatures creates a new UPFunctionFeatures IE.
// Each feature should be given by octets (5th to 8th octet). It expects 4 octets
// as input, excessive ones are ignored.
func NewUPFunctionFeatures(features ...uint8) *IE {
	var l int
	if len(features) >= 3 {
		l = 4
	} else {
		l = 2
	}

	ie := New(UPFunctionFeatures, make([]byte, l))
	for i, feature := range features {
		if i > 3 {
			break
		}
		ie.Payload[i] = feature
	}

	return ie
}

// UPFunctionFeatures returns UPFunctionFeatures in []byte if the type of IE matches.
func (i *IE) UPFunctionFeatures() ([]byte, error) {
	if i.Type != UPFunctionFeatures {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// HasBUCP reports whether an IE has BUCP bit.
func (i *IE) HasBUCP() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has1stBit(i.Payload[0])
}

// HasDDND reports whether an IE has DDND bit.
func (i *IE) HasDDND() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has2ndBit(i.Payload[0])
}

// HasDLBD reports whether an IE has DLBD bit.
func (i *IE) HasDLBD() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has3rdBit(i.Payload[0])
}

// HasTRST reports whether an IE has TRST bit.
func (i *IE) HasTRST() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has4thBit(i.Payload[0])
}

// HasFTUP reports whether an IE has FTUP bit.
func (i *IE) HasFTUP() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has5thBit(i.Payload[0])
}

// HasPFDM reports whether an IE has PFDM bit.
func (i *IE) HasPFDM() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has6thBit(i.Payload[0])
}

// HasHEEU reports whether an IE has HEEU bit.
func (i *IE) HasHEEU() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has7thBit(i.Payload[0])
}

// HasTREU reports whether an IE has TREU bit.
func (i *IE) HasTREU() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 1 {
		return false
	}

	return has8thBit(i.Payload[0])
}

// HasEMPU reports whether an IE has EMPU bit.
func (i *IE) HasEMPU() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has1stBit(i.Payload[1])
}

// HasPDIU reports whether an IE has PDIU bit.
func (i *IE) HasPDIU() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has2ndBit(i.Payload[1])
}

// HasUDBC reports whether an IE has UDBC bit.
func (i *IE) HasUDBC() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has3rdBit(i.Payload[1])
}

// HasQUOAC reports whether an IE has QUOAC bit.
func (i *IE) HasQUOAC() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has4thBit(i.Payload[1])
}

// HasTRACE reports whether an IE has TRACE bit.
func (i *IE) HasTRACE() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has5thBit(i.Payload[1])
}

// HasFRRT reports whether an IE has FRRT bit.
func (i *IE) HasFRRT() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has6thBit(i.Payload[1])
}

// HasPFDE reports whether an IE has PFDE bit.
func (i *IE) HasPFDE() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 2 {
		return false
	}

	return has7thBit(i.Payload[1])
}

// HasEPFAR reports whether an IE has EPFAR bit.
func (i *IE) HasEPFAR() bool {
	switch i.Type {
	case UPFunctionFeatures:
		if len(i.Payload) < 2 {
			return false
		}

		return has8thBit(i.Payload[1])
	case CPFunctionFeatures:
		if len(i.Payload) < 1 {
			return false
		}

		return has3rdBit(i.Payload[0])
	default:
		return false
	}
}

// HasDPDRA reports whether an IE has DPDRA  bit.
func (i *IE) HasDPDRA() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 3 {
		return false
	}

	return has1stBit(i.Payload[2])
}

// HasADPDP reports whether an IE has ADPDP  bit.
func (i *IE) HasADPDP() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 3 {
		return false
	}

	return has2ndBit(i.Payload[2])
}

// HasUEIP reports whether an IE has UEIP  bit.
func (i *IE) HasUEIP() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 3 {
		return false
	}

	return has3rdBit(i.Payload[2])
}

// HasSSET reports whether an IE has SSET  bit.
func (i *IE) HasSSET() bool {
	switch i.Type {
	case UPFunctionFeatures:
		if len(i.Payload) < 3 {
			return false
		}

		return has4thBit(i.Payload[2])
	case CPFunctionFeatures:
		if len(i.Payload) < 1 {
			return false
		}

		return has4thBit(i.Payload[0])
	default:
		return false
	}
}

// HasMNOP reports whether an IE has MNOP  bit.
func (i *IE) HasMNOP() bool {
	switch i.Type {
	case UPFunctionFeatures:
		if len(i.Payload) < 3 {
			return false
		}

		return has5thBit(i.Payload[2])
	case MeasurementInformation:
		if len(i.Payload) < 1 {
			return false
		}

		return has5thBit(i.Payload[0])
	default:
		return false
	}
}

// HasMTE reports whether an IE has MTE  bit.
func (i *IE) HasMTE() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 3 {
		return false
	}

	return has6thBit(i.Payload[2])
}

// HasBUNDL reports whether an IE has BUNDL  bit.
func (i *IE) HasBUNDL() bool {
	switch i.Type {
	case UPFunctionFeatures:
		if len(i.Payload) < 3 {
			return false
		}

		return has7thBit(i.Payload[2])
	case CPFunctionFeatures:
		if len(i.Payload) < 1 {
			return false
		}

		return has5thBit(i.Payload[0])
	default:
		return false
	}
}

// HasGCOM reports whether an IE has GCOM  bit.
func (i *IE) HasGCOM() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 3 {
		return false
	}

	return has8thBit(i.Payload[2])
}

// HasMPAS reports whether an IE has MPAS bit.
func (i *IE) HasMPAS() bool {
	switch i.Type {
	case UPFunctionFeatures:
		if len(i.Payload) < 4 {
			return false
		}

		return has1stBit(i.Payload[3])
	case CPFunctionFeatures:
		if len(i.Payload) < 1 {
			return false
		}

		return has6thBit(i.Payload[0])
	default:
		return false
	}
}

// HasRTTL reports whether an IE has RTTL bit.
func (i *IE) HasRTTL() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 4 {
		return false
	}

	return has2ndBit(i.Payload[3])
}

// HasVTIME reports whether an IE has VTIME bit.
func (i *IE) HasVTIME() bool {
	if i.Type != UPFunctionFeatures {
		return false
	}
	if len(i.Payload) < 4 {
		return false
	}

	return has3rdBit(i.Payload[3])
}
