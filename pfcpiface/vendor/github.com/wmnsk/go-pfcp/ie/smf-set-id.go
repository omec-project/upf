// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewSMFSetID creates a new SMFSetID IE.
func NewSMFSetID(id string) *IE {
	l := len([]byte(id))
	i := New(SMFSetID, make([]byte, 1+l))

	i.Payload[0] = 0 // Spare
	copy(i.Payload[1:], []byte(id))

	return i
}

// SMFSetID returns SMFSetID in []byte if the type of IE matches.
func (i *IE) SMFSetID() ([]byte, error) {
	if i.Type != SMFSetID {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload, nil
}

// SMFSetIDString returns SMFSetID in string if the type of IE matches.
func (i *IE) SMFSetIDString() (string, error) {
	v, err := i.SMFSetID()
	if err != nil {
		return "", err
	}

	if len(v) < 1 {
		return "", io.ErrUnexpectedEOF
	}

	return string(v[1:]), nil
}
