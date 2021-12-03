// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"encoding/binary"
	"fmt"
)

func ConvertValueToBinary(value interface{}) ([]byte, error) {
	switch t := value.(type) {
	case []byte:
		return value.([]byte), nil
	case bool:
		uintFlag := uint8(0)
		flag := value.(bool)
		if flag {
			uintFlag = 1
		}
		b := make([]byte, 1)
		b[0] = uintFlag
		return b, nil
	case uint8:
		b := make([]byte, 1)
		b[0] = value.(uint8)
		return b, nil
	case uint16:
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, value.(uint16))
		return b, nil
	case uint32:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, value.(uint32))
		return b, nil
	case uint64:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, value.(uint64))
		return b, nil
	default:
		return nil, fmt.Errorf("failed to convert type %T to byte array", t)
	}
}
