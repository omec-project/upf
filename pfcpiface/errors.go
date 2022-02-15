// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Open Networking Foundation
package pfcpiface

import (
	"errors"
	"fmt"
)

var (
	errNotFound         = errors.New("not found")
	errInvalidArgument  = errors.New("invalid argument")
	errInvalidOperation = errors.New("invalid operation")
	errFailed           = errors.New("failed")
	errUnsupported      = errors.New("unsupported")
)

func ErrUnsupported(what string, value interface{}) error {
	return fmt.Errorf("%s=%v %w", what, value, errUnsupported)
}

func ErrNotFound(what string) error {
	return fmt.Errorf("%s %w", what, errNotFound)
}

func ErrNotFoundWithParam(what string, paramName string, paramValue interface{}) error {
	return fmt.Errorf("%s %w with %s=%v", what, errNotFound, paramName, paramValue)
}

func ErrInvalidOperation(operation interface{}) error {
	return fmt.Errorf("%w: %v", errInvalidOperation, operation)
}

func ErrInvalidArgument(name string, value interface{}) error {
	return fmt.Errorf("%w '%s': %v", errInvalidArgument, name, value)
}

func ErrInvalidArgumentWithReason(name string, value interface{}, reason string) error {
	return fmt.Errorf("%w '%s'=%v (%s)", errInvalidArgument, name, value, reason)
}

func ErrOperationFailedWithReason(operation interface{}, reason string) error {
	return fmt.Errorf("%v %w due to: : %s", operation, errFailed, reason)
}

func ErrOperationFailedWithParam(operation interface{}, paramName string, paramValue interface{}) error {
	return fmt.Errorf("'%v' %w for %s=%v", operation, errFailed, paramName, paramValue)
}
