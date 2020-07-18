// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
)

// Node-ID Type definitions.
const (
	nodeIDIPv4 uint8 = iota
	nodeIDIPv6
	nodeIDOther
)

// NewFQCSID creates a new FQCSID IE.
func NewFQCSID(nodeAddr string, csIDs ...uint16) *IE {
	var (
		nid   []byte
		ntype uint8
	)
	ip := net.ParseIP(nodeAddr)
	if ip == nil {
		var err error
		nid, err = hex.DecodeString(nodeAddr)
		if err != nil {
			return nil
		}
		ntype = nodeIDOther
	} else if v4 := ip.To4(); v4 != nil {
		nid = v4
		ntype = nodeIDIPv4
	} else {
		nid = ip
		ntype = nodeIDIPv6
	}

	i := New(FQCSID, make([]byte, 1+len(nid)+len(csIDs)*2))
	i.Payload[0] = ((ntype << 4) & 0xf0) | uint8(len(csIDs)&0x0f)

	offset := len(nid) + 1
	copy(i.Payload[1:offset], nid)
	for n, csid := range csIDs {
		binary.BigEndian.PutUint16(i.Payload[offset+n*2:offset+n*2+2], csid)
	}
	return i
}

// FQCSID returns FQCSID in []byte if the type of IE matches.
func (i *IE) FQCSID() ([]byte, error) {
	if i.Type != FQCSID {
		return nil, &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 3 {
		return nil, io.ErrUnexpectedEOF
	}

	return i.Payload, nil
}

// NodeIDType returns NodeIDType in uint8 if the type of IE matches.
func (i *IE) NodeIDType() (uint8, error) {
	if len(i.Payload) == 0 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case FQCSID:
		return (i.Payload[0] >> 4) & 0x0f, nil
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}

// NodeAddress returns NodeAddress in []byte if the type of IE matches.
func (i *IE) NodeAddress() ([]byte, error) {
	if len(i.Payload) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case FQCSID:
		switch (i.Payload[0] >> 4) & 0x0f {
		case nodeIDIPv4, nodeIDOther:
			if len(i.Payload) < 6 {
				return nil, io.ErrUnexpectedEOF
			}
			return i.Payload[1:5], nil
		case nodeIDIPv6:
			if len(i.Payload) < 18 {
				return nil, io.ErrUnexpectedEOF
			}
			return i.Payload[1:17], nil
		default:
			return nil, ErrMalformed
		}
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// CSIDs returns CSIDs in []uint16 if the type of IE matches.
func (i *IE) CSIDs() ([]uint16, error) {
	if len(i.Payload) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case FQCSID:
		offset := 0
		switch (i.Payload[0] >> 4) & 0x0f {
		case nodeIDIPv4, nodeIDOther:
			offset += 5
		case nodeIDIPv6:
			offset += 17
		default:
			return nil, ErrMalformed
		}

		var csids []uint16
		for {
			if offset+2 > len(i.Payload) {
				break
			}
			csids = append(csids, binary.BigEndian.Uint16(i.Payload[offset:offset+2]))
			offset += 2
		}
		return csids, nil
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}
