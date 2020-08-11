// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewPacketReplicationAndDetectionCarryOnInformation creates a new PacketReplicationAndDetectionCarryOnInformation IE.
func NewPacketReplicationAndDetectionCarryOnInformation(flag uint8) *IE {
	return newUint8ValIE(PacketReplicationAndDetectionCarryOnInformation, flag)
}

// PacketReplicationAndDetectionCarryOnInformation returns PacketReplicationAndDetectionCarryOnInformation in []byte if the type of IE matches.
func (i *IE) PacketReplicationAndDetectionCarryOnInformation() ([]byte, error) {
	if len(i.Payload) < 1 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case PacketReplicationAndDetectionCarryOnInformation:
		return i.Payload, nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PacketReplicationAndDetectionCarryOnInformation {
				return x.PacketReplicationAndDetectionCarryOnInformation()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// HasPRIUEAI reports whether an IE has PRIUEAI bit.
func (i *IE) HasPRIUEAI() bool {
	v, err := i.PacketReplicationAndDetectionCarryOnInformation()
	if err != nil {
		return false
	}

	return has1stBit(v[0])
}

// HasPRINT19I reports whether an IE has PRINT19I bit.
func (i *IE) HasPRINT19I() bool {
	v, err := i.PacketReplicationAndDetectionCarryOnInformation()
	if err != nil {
		return false
	}

	return has2ndBit(v[0])
}

// HasPRIN6I reports whether an IE has PRIN6I bit.
func (i *IE) HasPRIN6I() bool {
	v, err := i.PacketReplicationAndDetectionCarryOnInformation()
	if err != nil {
		return false
	}

	return has3rdBit(v[0])
}

// HasDCARONI reports whether an IE has DCARONI bit.
func (i *IE) HasDCARONI() bool {
	v, err := i.PacketReplicationAndDetectionCarryOnInformation()
	if err != nil {
		return false
	}

	return has4thBit(v[0])
}
