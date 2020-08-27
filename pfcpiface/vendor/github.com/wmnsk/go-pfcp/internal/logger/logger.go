// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

// Package logger provides a logging functionalities for go-pfcp.
//
// This is hidden here to be able to be imported from each package of go-pfcp.
package logger

import (
	"io/ioutil"
	"log"
	"os"
	"sync"
)

var (
	logger = log.New(os.Stderr, "", log.LstdFlags)
	logMu  sync.Mutex
)

// SetLogger replaces the standard logger with arbitrary *log.Logger.
//
// DON'T CALL THIS. Use the func in pfcp package instead.
//
// This package prints just informational logs from goroutines working background
// that might help developers test the program but can be ignored safely. More
// important ones that needs any action by caller would be returned as errors.
func SetLogger(l *log.Logger) {
	if l == nil {
		log.Println("Don't pass nil to SetLogger: use DisableLogging instead.")
	}

	setLogger(l)
}

// EnableLogging enables the logging from the package.
//
// DON'T CALL THIS. Use the func in pfcp package instead.
//
// If l is nil, it uses default logger provided by the package.
// Logging is enabled by default.
//
// See also: SetLogger.
func EnableLogging(l *log.Logger) {
	logMu.Lock()
	defer logMu.Unlock()

	setLogger(l)
}

// DisableLogging disables the logging from the package.
//
// DON'T CALL THIS. Use the func in pfcp package instead.
//
// Logging is enabled by default.
func DisableLogging() {
	logMu.Lock()
	defer logMu.Unlock()

	logger.SetOutput(ioutil.Discard)
}

func setLogger(l *log.Logger) {
	if l == nil {
		l = log.New(os.Stderr, "", log.LstdFlags)
	}

	logMu.Lock()
	defer logMu.Unlock()

	logger = l
}

func Logf(format string, v ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()

	logger.Printf(format, v...)
}
