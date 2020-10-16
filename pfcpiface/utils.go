// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/binary"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
)

func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	/*
		// For info on each, see: https://golang.org/pkg/runtime/#MemStats
		log.Printf("Alloc = %v MiB, %v", bToMb(m.Alloc), m.Alloc)
		log.Printf("MAllocs = %v", m.Mallocs)
		log.Printf("Frees = %v", m.Frees)
		log.Printf("\tTotalAlloc = %v MiB, %v", bToMb(m.TotalAlloc), m.TotalAlloc)
		log.Printf("\tSys = %v MiBi, %v", bToMb(m.Sys), m.Sys)
		log.Printf("\tNumGC = %v\n", m.NumGC)*/
}

func has2ndBit(f uint8) bool {
	return (f&0x02)>>1 == 1
}

func has1stBit(f uint8) bool {
	return (f & 0x01) == 1
}

/*
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func deleteFromSlice([]r) {
    for _, x := range s {
        if isValid(x) {
            // copy and increment index
            s[i] = x
            i++
        }
    }
    // Prevent memory leak by erasing truncated values
    // (not needed if values don't contain pointers, directly or indirectly)
    for j := i; j < len(s); j++ {
        s[j] = nil
    }
    s = s[:i]
}*/

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

func mySEID(peerSEID uint64) uint64 {
	return (peerSEID >> 2)
}

func peerSEID(mySEID uint64) uint64 {
	return (mySEID << 2)
}
