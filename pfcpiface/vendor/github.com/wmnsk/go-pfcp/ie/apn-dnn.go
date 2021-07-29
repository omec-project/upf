// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"github.com/wmnsk/go-pfcp/internal/utils"
)

// NewAPNDNN creates a new APNDNN IE.
func NewAPNDNN(apn string) *IE {
	return newFQDNIE(APNDNN, apn)
}

// APNDNN returns APNDNN in string if the type of IE matches.
func (i *IE) APNDNN() (string, error) {
	if i.Type != APNDNN {
		return "", &InvalidTypeError{Type: i.Type}
	}

	return utils.DecodeFQDN(i.Payload), nil
}

// MustAPNDNN returns APNDNN in string, ignoring errors.
// This should only be used if it is assured to have the value.
func (i *IE) MustAPNDNN() string {
	v, _ := i.APNDNN()
	return v
}
