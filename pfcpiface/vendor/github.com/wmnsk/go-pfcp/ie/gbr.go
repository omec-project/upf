// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
    "log"
	"io"
)

// NewGBR creates a new GBR IE.
func NewGBR(ul, dl uint32) *IE {
	i := New(GBR, make([]byte, 8))

	binary.BigEndian.PutUint32(i.Payload[0:4], ul)
	binary.BigEndian.PutUint32(i.Payload[4:8], dl)

	return i
}

// GBR returns GBR in []byte if the type of IE matches.
func (i *IE) GBR() ([]byte, error) {
	if len(i.Payload) < 8 {
		return nil, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case GBR:
		return i.Payload, nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == GBR {
				return x.GBR()
			}
		}
		return nil, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == GBR {
				return x.GBR()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// GBRUL returns GBRUL in uint64 if the type of IE matches.
func (i *IE) GBRUL() (uint64, error) {
	v, err := i.GBR()
	if err != nil {
		return 0, err
	}

    var mySlice = []byte{0, 0, 0, v[0], v[1], v[2], v[3], v[4]}
	num := binary.BigEndian.Uint64(mySlice)
    log.Println(num)
    return num, nil
}

// GBRDL returns GBRDL in uint64 if the type of IE matches.
func (i *IE) GBRDL() (uint64, error) {
	v, err := i.GBR()
	if err != nil {
		return 0, err
	}

    var mySlice = []byte{0, 0, 0, v[5], v[6], v[7], v[8], v[9]}
	num := binary.BigEndian.Uint64(mySlice)
    log.Println(num)
    return num, nil
}
