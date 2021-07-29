// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"

	"github.com/wmnsk/go-pfcp/internal/utils"
)

// NewGBR creates a new GBR IE.
func NewGBR(ul, dl uint64) *IE {
	i := New(GBR, make([]byte, 10))
	copy(i.Payload[0:5], utils.Uint64To40(ul))
	copy(i.Payload[5:10], utils.Uint64To40(dl))
	return i
}

// GBR returns GBR in []byte if the type of IE matches.
func (i *IE) GBR() ([]byte, error) {
	if len(i.Payload) < 10 {
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
	return utils.Uint40To64(v[0:5]), nil
}

// GBRDL returns GBRDL in uint64 if the type of IE matches.
func (i *IE) GBRDL() (uint64, error) {
	v, err := i.GBR()
	if err != nil {
		return 0, err
	}
	return utils.Uint40To64(v[5:10]), nil
}
