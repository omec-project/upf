// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import "encoding/hex"

func uint24To32(b []byte) uint32 {
	if len(b) != 3 {
		return 0
	}
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func uint32To24(n uint32) []byte {
	return []byte{uint8(n >> 16), uint8(n >> 8), uint8(n)}
}

func strToSwappedBytes(s, filler string) ([]byte, error) {
	var raw []byte
	var err error
	if len(s)%2 == 0 {
		raw, err = hex.DecodeString(s)
	} else {
		raw, err = hex.DecodeString(s + filler)
	}
	if err != nil {
		return nil, err
	}

	return swap(raw), nil
}

func swappedBytesToStr(raw []byte, cutLastDigit bool) string {
	if len(raw) == 0 {
		return ""
	}

	s := hex.EncodeToString(swap(raw))
	if cutLastDigit {
		s = s[:len(s)-1]
	}

	return s
}

func swap(raw []byte) []byte {
	var swapped []byte
	for n := range raw {
		t := ((raw[n] >> 4) & 0xf) + ((raw[n] << 4) & 0xf0)
		swapped = append(swapped, t)
	}
	return swapped
}

func has8thBit(f uint8) bool {
	return (f&0x80)>>7 == 1
}

func has7thBit(f uint8) bool {
	return (f&0x40)>>6 == 1
}

func has6thBit(f uint8) bool {
	return (f&0x20)>>5 == 1
}

func has5thBit(f uint8) bool {
	return (f&0x010)>>4 == 1
}

func has4thBit(f uint8) bool {
	return (f&0x08)>>3 == 1
}

func has3rdBit(f uint8) bool {
	return (f&0x04)>>2 == 1
}

func has2ndBit(f uint8) bool {
	return (f&0x02)>>1 == 1
}

func has1stBit(f uint8) bool {
	return (f & 0x01) == 1
}
