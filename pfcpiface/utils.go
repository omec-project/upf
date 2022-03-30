// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"

	"github.com/omec-project/upf-epc/internal/p4constants"

	log "github.com/sirupsen/logrus"
)

// Bits type.
type Bits uint8

// Set Bits.
func Set(b, flag Bits) Bits { return b | flag }

// func Clear(b, flag Bits) Bits  { return b &^ flag }
// func Toggle(b, flag Bits) Bits { return b ^ flag }
// func Has(b, flag Bits) bool { return b&flag != 0 }

func setUeipFeature(features ...uint8) {
	if len(features) >= 3 {
		features[2] = features[2] | 0x04
	}
}

func setEndMarkerFeature(features ...uint8) {
	if len(features) >= 2 {
		features[1] = features[1] | 0x01
	}
}

func has2ndBit(f uint8) bool {
	return (f&0x02)>>1 == 1
}

func has5thBit(f uint8) bool {
	return (f & 0x010) == 1
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

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

func maxUint64(x, y uint64) uint64 {
	if x < y {
		return y
	}

	return x
}

// Returns the bandwidth delay product for a given rate in kbps and duration in ms.
func calcBurstSizeFromRate(kbps uint64, ms uint64) uint64 {
	return uint64((float64(kbps) * 1000 / 8) * (float64(ms) / 1000))
}

// MustParseStrIP : parse IP address from config and fail on error.
func MustParseStrIP(address string) *net.IPNet {
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		log.Fatalf("unable to parse IP %v that we should parse", address)
	}

	log.Info("Parsed IP: ", ip)

	return ipNet
}

// GetUnicastAddressFromInterface returns a unicast IP address configured on the interface.
func GetUnicastAddressFromInterface(interfaceName string) (net.IP, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}

	addresses, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	ip, _, err := net.ParseCIDR(addresses[0].String())
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func GetSliceTCMeterIndex(sliceID uint8, TC uint8) (int64, error) {
	if sliceID >= (1 << p4constants.BitwidthMfSliceId) {
		return 0, ErrInvalidArgumentWithReason("SliceID", sliceID, "Slice ID higher than max supported slice ID")
	}

	if TC >= (1 << p4constants.BitwidthApTc) {
		return 0, ErrInvalidArgumentWithReason("TC", TC, "TC higher than max supported Traffic Class")
	}

	return int64((sliceID << 2) + (TC & 0b11)), nil
}
