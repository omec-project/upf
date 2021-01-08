// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"strings"
)

//grpc channel state
const (
	Ready = 2 //grpc channel state Ready
)

type Bits uint8

func Set(b, flag Bits) Bits { return b | flag }

//func Clear(b, flag Bits) Bits  { return b &^ flag }
//func Toggle(b, flag Bits) Bits { return b ^ flag }
//func Has(b, flag Bits) bool { return b&flag != 0 }

func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func ipMask2int(ip net.IPMask) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func hex2int(hexStr string) uint32 {
	// remove 0x suffix if found in the input string
	cleaned := strings.Replace(hexStr, "0x", "", -1)

	// base 16 for hexadecimal
	result, _ := strconv.ParseUint(cleaned, 16, 32)
	return uint32(result)
}

func int2ip(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}

func getRemoteIP(dstIP string) net.IP {
	conn, err := net.Dial("udp", dstIP+":"+PFCPPort)
	if err != nil {
		ip := "0.0.0.0"
		return net.ParseIP(ip)
	}
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().(*net.UDPAddr)

	return remoteAddr.IP
}

func getLocalIP(dstIP string) net.IP {
	conn, err := net.Dial("udp", dstIP+":"+PFCPPort)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}
