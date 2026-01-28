// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"encoding/binary"

	"github.com/omec-project/upf-epc/logger"
)

// CreateFAR appends far to existing list of FARs in the session.
func (s *PFCPSession) CreateFAR(f far) {
	s.fars = append(s.fars, f)
}

// Packet structure constants for GTP-U end marker
const (
	ethHeaderSize  = 14
	ipv4HeaderSize = 20
	udpHeaderSize  = 8
	gtpHeaderSize  = 8 // Minimum GTP-U header without optional fields
	endMarkerSize  = ethHeaderSize + ipv4HeaderSize + udpHeaderSize + gtpHeaderSize

	// Offsets within the packet
	ethTypeOffset    = 12
	ipOffset         = ethHeaderSize
	ipChecksumOffset = ipOffset + 10
	ipSrcOffset      = ipOffset + 12
	ipDstOffset      = ipOffset + 16
	udpOffset        = ipOffset + ipv4HeaderSize
	gtpOffset        = udpOffset + udpHeaderSize

	// Protocol constants
	gtpEndMarkerType = 254
)

// Placeholder MAC addresses for end marker packets.
// These are immutable values that will be replaced by the dataplane (BESS)
// with actual MAC addresses based on the routing/forwarding table.
// Note: Defined as variables (not constants) because Go doesn't support
// byte slice/array constants, but these should be treated as read-only.
var (
	endMarkerDstMAC = [6]byte{0xBD, 0xBD, 0xBD, 0xBD, 0xBD, 0xBD} // Placeholder for next-hop MAC
	endMarkerSrcMAC = [6]byte{0xFF, 0xAA, 0xFA, 0xAA, 0xFF, 0xAA} // Placeholder for local interface MAC
)

func addEndMarker(farItem far, endMarkerList *[][]byte) {
	logger.PfcpLog.Infoln("adding end marker for farID:", farItem.farID)

	packet := make([]byte, endMarkerSize)

	// Ethernet header - placeholder MAC addresses will be replaced by dataplane
	copy(packet[0:6], endMarkerDstMAC[:])
	copy(packet[6:12], endMarkerSrcMAC[:])
	binary.BigEndian.PutUint16(packet[ethTypeOffset:], 0x0800) // IPv4

	// IPv4 header
	packet[ipOffset] = 0x45                                                                     // Version 4, IHL 5
	packet[ipOffset+1] = 0                                                                      // DSCP/ECN
	binary.BigEndian.PutUint16(packet[ipOffset+2:], ipv4HeaderSize+udpHeaderSize+gtpHeaderSize) // Total length
	binary.BigEndian.PutUint16(packet[ipOffset+4:], 0)                                          // Identification
	binary.BigEndian.PutUint16(packet[ipOffset+6:], 0)                                          // Flags/Fragment offset
	packet[ipOffset+8] = 64                                                                     // TTL
	packet[ipOffset+9] = 17                                                                     // UDP protocol
	// Checksum at ipOffset+10 calculated below
	binary.BigEndian.PutUint32(packet[ipSrcOffset:], farItem.tunnelIP4Src)
	binary.BigEndian.PutUint32(packet[ipDstOffset:], farItem.tunnelIP4Dst)

	// Calculate and set IPv4 checksum
	binary.BigEndian.PutUint16(packet[ipChecksumOffset:], calculateIPv4Checksum(packet[ipOffset:ipOffset+ipv4HeaderSize]))

	// UDP header
	binary.BigEndian.PutUint16(packet[udpOffset:], tunnelGTPUPort)                // Source port
	binary.BigEndian.PutUint16(packet[udpOffset+2:], tunnelGTPUPort)              // Destination port
	binary.BigEndian.PutUint16(packet[udpOffset+4:], udpHeaderSize+gtpHeaderSize) // UDP length
	binary.BigEndian.PutUint16(packet[udpOffset+6:], 0)                           // Checksum (optional for IPv4)

	// GTP-U header (8 bytes - no optional fields for end marker)
	packet[gtpOffset] = 0x30                                             // Version 1, PT=1, no extension/sequence/N-PDU
	packet[gtpOffset+1] = gtpEndMarkerType                               // Message type: End Marker
	binary.BigEndian.PutUint16(packet[gtpOffset+2:], 0)                  // Message length: 0
	binary.BigEndian.PutUint32(packet[gtpOffset+4:], farItem.tunnelTEID) // TEID

	*endMarkerList = append(*endMarkerList, packet)
}

// calculateIPv4Checksum calculates Internet Checksum for IPv4 header (RFC 1071).
// This function handles fixed 20-byte IPv4 headers (no options) by summing all 10 16-bit words directly.
func calculateIPv4Checksum(header []byte) uint16 {
	if len(header) < ipv4HeaderSize {
		logger.PfcpLog.Errorf("invalid IPv4 header length for checksum calculation: expected %d bytes, got %d bytes",
			ipv4HeaderSize, len(header))
		return 0
	}

	// Sum all 16-bit words (20 bytes = 10 words)
	var sum uint32
	for i := 0; i < ipv4HeaderSize; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i:]))
	}

	// Fold 32-bit sum to 16 bits
	sum = (sum >> 16) + (sum & 0xffff)
	sum += sum >> 16
	return ^uint16(sum)
}

// UpdateFAR updates existing far in the session.
func (s *PFCPSession) UpdateFAR(f *far, endMarkerList *[][]byte) error {
	for idx, v := range s.fars {
		if v.farID == f.farID {
			if f.sendEndMarker {
				addEndMarker(v, endMarkerList)
			}

			s.fars[idx] = *f

			return nil
		}
	}

	return ErrNotFound("FAR")
}

// RemoveFAR removes far from existing list of FARs in the session.
func (s *PFCPSession) RemoveFAR(id uint32) (*far, error) {
	for idx, v := range s.fars {
		if v.farID == id {
			s.fars = append(s.fars[:idx], s.fars[idx+1:]...)
			return &v, nil
		}
	}

	return nil, ErrNotFound("FAR")
}
