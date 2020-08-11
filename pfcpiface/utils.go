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

func ip2int(ip net.IP) uint32 {
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

func getOutboundIP(dstIP string) (net.IP, net.IP)  {
	log.Println("SPGWC address IP: ", dstIP)
	conn, err := net.Dial("udp", dstIP+":"+PFCPPort)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
    remoteAddr := conn.RemoteAddr().(*net.UDPAddr)
	log.Println("SPGWC address IP: ", remoteAddr.IP.String())
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, remoteAddr.IP
}

func mySEID(peerSEID uint64) uint64 {
	return (peerSEID >> 2)
}

func peerSEID(mySEID uint64) uint64 {
	return (mySEID << 2)
}
