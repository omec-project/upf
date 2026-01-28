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

func addEndMarker(farItem far, endMarkerList *[][]byte) {
	logger.PfcpLog.Infoln("adding end marker for farID:", farItem.farID)

	packet := make([]byte, endMarkerSize)

	// Ethernet header - using direct indexing for better performance
	packet[0], packet[1], packet[2], packet[3], packet[4], packet[5] = 0xBD, 0xBD, 0xBD, 0xBD, 0xBD, 0xBD
	packet[6], packet[7], packet[8], packet[9], packet[10], packet[11] = 0xFF, 0xAA, 0xFA, 0xAA, 0xFF, 0xAA
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

	// Calculate and set IPv4 checksum (optimized for 20-byte header)
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

// calculateIPv4Checksum calculates Internet Checksum for IPv4 header (RFC 1071)
// Optimized for fixed 20-byte IPv4 header by unrolling the loop
func calculateIPv4Checksum(header []byte) uint16 {
	// Unroll loop for 10 16-bit words (20 bytes)
	sum := uint32(header[0])<<8 | uint32(header[1]) |
		uint32(header[2])<<8 | uint32(header[3]) |
		uint32(header[4])<<8 | uint32(header[5]) |
		uint32(header[6])<<8 | uint32(header[7]) |
		uint32(header[8])<<8 | uint32(header[9]) |
		uint32(header[10])<<8 | uint32(header[11]) |
		uint32(header[12])<<8 | uint32(header[13]) |
		uint32(header[14])<<8 | uint32(header[15]) |
		uint32(header[16])<<8 | uint32(header[17]) |
		uint32(header[18])<<8 | uint32(header[19])

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
