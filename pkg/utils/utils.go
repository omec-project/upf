// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022 Open Networking Foundation

package utils

import (
	"encoding/binary"
	"net"
)

func Uint32ToIp4(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)

	return ip
}

func Ip4ToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func MaxUint16(x, y uint16) uint16 {
	if x < y {
		return y
	}

	return x
}

func MinUint16(x, y uint16) uint16 {
	if MaxUint16(x, y) == x {
		return y
	}

	return x
}

func Uint8Has3rdBit(f uint8) bool {
	return (f&0x04)>>2 == 1
}

func Uint8Has2ndBit(f uint8) bool {
	return (f&0x02)>>1 == 1
}

func Uint8Has1stBit(f uint8) bool {
	return (f & 0x01) == 1
}
