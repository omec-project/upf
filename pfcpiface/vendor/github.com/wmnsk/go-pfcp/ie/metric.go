// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
)

// NewMetric creates a new Metric IE.
func NewMetric(metric uint8) *IE {
	return newUint8ValIE(Metric, metric)
}

// Metric returns Metric in uint8 if the type of IE matches.
func (i *IE) Metric() (uint8, error) {

	if len(i.Payload) < 1 {
		return 0, io.ErrUnexpectedEOF
	}

	switch i.Type {
	case Metric:
		return i.Payload[0], nil
	case LoadControlInformation:
		ies, err := i.LoadControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Metric {
				return x.Metric()
			}
		}
		return 0, ErrIENotFound
	case OverloadControlInformation:
		ies, err := i.OverloadControlInformation()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == Metric {
				return x.Metric()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
