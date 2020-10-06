// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"errors"
	"fmt"
)

// Error definitions.
var (
	ErrTooShortToParse = errors.New("too short to decode as GTP")
	ErrInvalidLength   = errors.New("length value is invalid")

	ErrInvalidType = errors.New("invalid type")
	ErrIENotFound  = errors.New("could not find the specified IE in a grouped IE")

	ErrMalformed = errors.New("malformed IE")

	ErrElementNotFound = errors.New("element not found")
)

// InvalidTypeError indicates the type of IE is invalid.
type InvalidTypeError struct {
	Type uint16
}

// Error returns message with the invalid type given.
func (e *InvalidTypeError) Error() string {
	return fmt.Sprintf("got invalid type: %d", e.Type)
}

// InvalidNodeIDError indicates the NodeID value is invalid.
type InvalidNodeIDError struct {
	ID uint8
}

// Error returns message with the invalid NodeID given.
func (e *InvalidNodeIDError) Error() string {
	return fmt.Sprintf("got invalid NodeID: %d", e.ID)
}
