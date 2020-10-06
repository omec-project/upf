// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
)

// NewQERCorrelationID creates a new QERCorrelationID IE.
func NewQERCorrelationID(id uint32) *IE {
	return newUint32ValIE(QERCorrelationID, id)
}

// QERCorrelationID returns QERCorrelationID in uint32 if the type of IE matches.
func (i *IE) QERCorrelationID() (uint32, error) {
	if len(i.Payload) < 4 {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	switch i.Type {
	case QERCorrelationID:
		return binary.BigEndian.Uint32(i.Payload[0:4]), nil
	case CreateQER:
		ies, err := i.CreateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERCorrelationID {
				return x.QERCorrelationID()
			}
		}
		return 0, ErrIENotFound
	case UpdateQER:
		ies, err := i.UpdateQER()
		if err != nil {
			return 0, err
		}
		for _, x := range ies {
			if x.Type == QERCorrelationID {
				return x.QERCorrelationID()
			}
		}
		return 0, ErrIENotFound
	default:
		return 0, &InvalidTypeError{Type: i.Type}
	}
}
