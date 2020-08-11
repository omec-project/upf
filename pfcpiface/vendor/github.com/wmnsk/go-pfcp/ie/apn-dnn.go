// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"strings"
)

// NewAPNDNN creates a new APNDNN IE.
func NewAPNDNN(apn string) *IE {
	i := New(APNDNN, make([]byte, len(apn)+1))
	var offset = 0
	for _, label := range strings.Split(apn, ".") {
		l := len(label)
		i.Payload[offset] = uint8(l)
		copy(i.Payload[offset+1:], []byte(label))
		offset += l + 1
	}

	return i
}

// APNDNN returns APNDNN in string if the type of IE matches.
func (i *IE) APNDNN() (string, error) {
	if i.Type != APNDNN {
		return "", &InvalidTypeError{Type: i.Type}
	}

	var (
		apn    []string
		offset int
	)
	max := len(i.Payload)
	for {
		if offset >= max {
			break
		}
		l := int(i.Payload[offset])
		if offset+l+1 > max {
			break
		}
		apn = append(apn, string(i.Payload[offset+1:offset+l+1]))
		offset += l + 1
	}

	return strings.Join(apn, "."), nil
}

// MustAPNDNN returns APNDNN in string, ignoring errors.
// This should only be used if it is assured to have the value.
func (i *IE) MustAPNDNN() string {
	v, _ := i.APNDNN()
	return v
}
