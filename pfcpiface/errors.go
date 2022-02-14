// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Open Networking Foundation
package main

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errNotFound         = errors.New("not found")
	errInvalidArgument  = errors.New("invalid argument")
	errInvalidOperation = errors.New("invalid operation")
	errFailed           = errors.New("failed")
)

type pfcpifaceError struct {
	message string
	error   []error
}

func (e *pfcpifaceError) unwrap() string {
	errMsg := strings.Builder{}
	errMsg.WriteString("")

	for i, e := range e.error {
		errMsg.WriteString(fmt.Sprintf("\n- Error %v: %v", i, e))
	}

	return errMsg.String()
}

func (e *pfcpifaceError) Error() string {
	return fmt.Sprintf("Message: %v. %v", e.message, e.unwrap())
}

func NewUnsupportedSourceIFaceError(iFaceType uint8, err ...error) *pfcpifaceError {
	msg := fmt.Sprintf("Unsupported source interface type. Provided: %v", iFaceType)
	return &pfcpifaceError{
		message: msg,
		error:   err,
	}
}

func NewUnsupportedPrecedenceValue(value uint32, err ...error) *pfcpifaceError {
	msg := fmt.Sprintf("Unsupported precedence greater than 65535. Provided: %v", value)
	return &pfcpifaceError{
		message: msg,
		error:   err,
	}
}

func NewNotFoundError(what string, err ...error) *pfcpifaceError {
	return &pfcpifaceError{
		message: fmt.Sprintf("%v not found", what),
		error:   err,
	}
}

func NewPDRNotFoundError(err ...error) *pfcpifaceError {
	return &pfcpifaceError{
		message: "PDR not found",
		error:   err,
	}
}

func NewQERNotFoundError(err ...error) *pfcpifaceError {
	return &pfcpifaceError{
		message: "QER not found",
		error:   err,
	}
}

func NewFARNotFoundError(err ...error) *pfcpifaceError {
	return &pfcpifaceError{
		message: "FAR not found",
		error:   err,
	}
}

func NewLoopbackInterfaceNotFoundError(err ...error) *pfcpifaceError {
	return &pfcpifaceError{
		message: "No loopback interface found",
		error:   err,
	}
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
