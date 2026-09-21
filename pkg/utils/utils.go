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
