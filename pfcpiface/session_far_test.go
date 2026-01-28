// SPDX-FileCopyrightText: 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/binary"
	"net"
	"testing"
)

// TestUpdateFAR_WithEndMarkerEnabled tests UpdateFAR when sendEndMarker is true
func TestUpdateFAR_WithEndMarkerEnabled(t *testing.T) {
	// Setup: Create a session with an existing FAR
	session := &PFCPSession{
		localSEID:  100,
		remoteSEID: 200,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0),
			qers: make([]qer, 0),
			fars: []far{
				{
					farID:        1,
					fseID:        100,
					tunnelTEID:   0x12345678,
					tunnelIP4Src: ip2int(net.ParseIP("192.168.1.1")),
					tunnelIP4Dst: ip2int(net.ParseIP("10.0.0.1")),
					tunnelPort:   tunnelGTPUPort,
					applyAction:  ActionForward,
					dstIntf:      1,
				},
			},
		},
	}

	// Prepare updated FAR with sendEndMarker=true
	updatedFAR := &far{
		farID:         1,
		fseID:         100,
		sendEndMarker: true, // End marker enabled
		tunnelTEID:    0x87654321,
		tunnelIP4Src:  ip2int(net.ParseIP("192.168.2.1")),
		tunnelIP4Dst:  ip2int(net.ParseIP("10.0.0.2")),
		tunnelPort:    tunnelGTPUPort,
		applyAction:   ActionForward,
		dstIntf:       2,
	}

	endMarkerList := make([][]byte, 0)

	// Execute: Update the FAR
	err := session.UpdateFAR(updatedFAR, &endMarkerList)

	// Assert: No error should occur
	if err != nil {
		t.Fatalf("UpdateFAR failed: %v", err)
	}

	// Assert: End marker should be generated
	if len(endMarkerList) != 1 {
		t.Fatalf("expected 1 end marker, got %d", len(endMarkerList))
	}

	// Assert: End marker packet should have correct size
	packet := endMarkerList[0]
	if len(packet) != endMarkerSize {
		t.Errorf("expected end marker size %d, got %d", endMarkerSize, len(packet))
	}

	// Assert: Verify end marker uses OLD FAR tunnel information (before update)
	verifyEndMarkerPacket(t, packet, 0x12345678, "192.168.1.1", "10.0.0.1")

	// Assert: FAR should be updated in session with NEW values
	if len(session.fars) != 1 {
		t.Fatalf("expected 1 FAR in session, got %d", len(session.fars))
	}

	updatedSessionFAR := session.fars[0]
	if updatedSessionFAR.tunnelTEID != 0x87654321 {
		t.Errorf("expected updated TEID 0x87654321, got 0x%x", updatedSessionFAR.tunnelTEID)
	}
	if updatedSessionFAR.tunnelIP4Src != ip2int(net.ParseIP("192.168.2.1")) {
		t.Errorf("expected updated source IP, got %v", int2ip(updatedSessionFAR.tunnelIP4Src))
	}
	if updatedSessionFAR.tunnelIP4Dst != ip2int(net.ParseIP("10.0.0.2")) {
		t.Errorf("expected updated dest IP, got %v", int2ip(updatedSessionFAR.tunnelIP4Dst))
	}
}

// TestUpdateFAR_WithEndMarkerDisabled tests UpdateFAR when sendEndMarker is false
func TestUpdateFAR_WithEndMarkerDisabled(t *testing.T) {
	// Setup: Create a session with an existing FAR
	session := &PFCPSession{
		localSEID:  100,
		remoteSEID: 200,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0),
			qers: make([]qer, 0),
			fars: []far{
				{
					farID:        2,
					fseID:        100,
					tunnelTEID:   0xAABBCCDD,
					tunnelIP4Src: ip2int(net.ParseIP("172.16.1.1")),
					tunnelIP4Dst: ip2int(net.ParseIP("10.1.1.1")),
					tunnelPort:   tunnelGTPUPort,
					applyAction:  ActionForward,
				},
			},
		},
	}

	// Prepare updated FAR with sendEndMarker=false (default)
	updatedFAR := &far{
		farID:         2,
		fseID:         100,
		sendEndMarker: false, // End marker disabled
		tunnelTEID:    0x11223344,
		tunnelIP4Src:  ip2int(net.ParseIP("172.16.2.1")),
		tunnelIP4Dst:  ip2int(net.ParseIP("10.1.1.2")),
		tunnelPort:    tunnelGTPUPort,
		applyAction:   ActionForward,
	}

	endMarkerList := make([][]byte, 0)

	// Execute: Update the FAR
	err := session.UpdateFAR(updatedFAR, &endMarkerList)

	// Assert: No error should occur
	if err != nil {
		t.Fatalf("UpdateFAR failed: %v", err)
	}

	// Assert: No end marker should be generated
	if len(endMarkerList) != 0 {
		t.Errorf("expected 0 end markers when disabled, got %d", len(endMarkerList))
	}

	// Assert: FAR should still be updated in session
	if len(session.fars) != 1 {
		t.Fatalf("expected 1 FAR in session, got %d", len(session.fars))
	}

	updatedSessionFAR := session.fars[0]
	if updatedSessionFAR.tunnelTEID != 0x11223344 {
		t.Errorf("expected updated TEID 0x11223344, got 0x%x", updatedSessionFAR.tunnelTEID)
	}
}

// TestUpdateFAR_NonExistentFAR tests UpdateFAR with a FAR ID that does not exist
func TestUpdateFAR_NonExistentFAR(t *testing.T) {
	// Setup: Create a session with a FAR
	session := &PFCPSession{
		localSEID:  100,
		remoteSEID: 200,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0),
			qers: make([]qer, 0),
			fars: []far{
				{
					farID: 1,
				},
			},
		},
	}

	// Prepare FAR with non-existent ID
	updatedFAR := &far{
		farID:         999, // This FAR ID does not exist
		sendEndMarker: true,
	}

	endMarkerList := make([][]byte, 0)

	// Execute: Attempt to update non-existent FAR
	err := session.UpdateFAR(updatedFAR, &endMarkerList)

	// Assert: Should return ErrNotFound error
	if err == nil {
		t.Fatal("expected error for non-existent FAR, got nil")
	}

	// Assert: No end marker should be generated
	if len(endMarkerList) != 0 {
		t.Errorf("expected 0 end markers for non-existent FAR, got %d", len(endMarkerList))
	}

	// Assert: Original FAR should remain unchanged
	if len(session.fars) != 1 || session.fars[0].farID != 1 {
		t.Error("Original FAR should remain unchanged")
	}
}

// TestUpdateFAR_MultipleFARs tests UpdateFAR in a session with multiple FARs
func TestUpdateFAR_MultipleFARs(t *testing.T) {
	// Setup: Create a session with multiple FARs
	session := &PFCPSession{
		localSEID:  100,
		remoteSEID: 200,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0),
			qers: make([]qer, 0),
			fars: []far{
				{
					farID:        1,
					tunnelTEID:   0x1111,
					tunnelIP4Src: ip2int(net.ParseIP("1.1.1.1")),
					tunnelIP4Dst: ip2int(net.ParseIP("2.2.2.2")),
				},
				{
					farID:        2,
					tunnelTEID:   0x2222,
					tunnelIP4Src: ip2int(net.ParseIP("3.3.3.3")),
					tunnelIP4Dst: ip2int(net.ParseIP("4.4.4.4")),
				},
				{
					farID:        3,
					tunnelTEID:   0x3333,
					tunnelIP4Src: ip2int(net.ParseIP("5.5.5.5")),
					tunnelIP4Dst: ip2int(net.ParseIP("6.6.6.6")),
				},
			},
		},
	}

	// Update the middle FAR with end marker
	updatedFAR := &far{
		farID:         2,
		sendEndMarker: true,
		tunnelTEID:    0xFFFF,
	}

	endMarkerList := make([][]byte, 0)

	// Execute
	err := session.UpdateFAR(updatedFAR, &endMarkerList)

	// Assert
	if err != nil {
		t.Fatalf("UpdateFAR failed: %v", err)
	}

	// Assert: One end marker generated with old TEID
	if len(endMarkerList) != 1 {
		t.Fatalf("expected 1 end marker, got %d", len(endMarkerList))
	}

	verifyEndMarkerPacket(t, endMarkerList[0], 0x2222, "3.3.3.3", "4.4.4.4")

	// Assert: Only the target FAR is updated
	if session.fars[0].farID != 1 || session.fars[0].tunnelTEID != 0x1111 {
		t.Error("FAR 1 should remain unchanged")
	}
	if session.fars[1].farID != 2 || session.fars[1].tunnelTEID != 0xFFFF {
		t.Error("FAR 2 should be updated")
	}
	if session.fars[2].farID != 3 || session.fars[2].tunnelTEID != 0x3333 {
		t.Error("FAR 3 should remain unchanged")
	}
}

// TestAddEndMarker_PacketStructure tests the structure of generated end marker packets
func TestAddEndMarker_PacketStructure(t *testing.T) {
	testCases := []struct {
		name         string
		farItem      far
		expectedTEID uint32
		expectedSrc  string
		expectedDst  string
	}{
		{
			name: "Standard end marker",
			farItem: far{
				farID:        10,
				tunnelTEID:   0xDEADBEEF,
				tunnelIP4Src: ip2int(net.ParseIP("192.168.100.1")),
				tunnelIP4Dst: ip2int(net.ParseIP("10.20.30.40")),
			},
			expectedTEID: 0xDEADBEEF,
			expectedSrc:  "192.168.100.1",
			expectedDst:  "10.20.30.40",
		},
		{
			name: "End marker with zero TEID",
			farItem: far{
				farID:        20,
				tunnelTEID:   0,
				tunnelIP4Src: ip2int(net.ParseIP("172.16.0.1")),
				tunnelIP4Dst: ip2int(net.ParseIP("192.168.0.1")),
			},
			expectedTEID: 0,
			expectedSrc:  "172.16.0.1",
			expectedDst:  "192.168.0.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endMarkerList := make([][]byte, 0)

			// Execute
			addEndMarker(tc.farItem, &endMarkerList)

			// Assert: One packet generated
			if len(endMarkerList) != 1 {
				t.Fatalf("expected 1 end marker, got %d", len(endMarkerList))
			}

			packet := endMarkerList[0]

			// Verify packet structure
			verifyEndMarkerPacket(t, packet, tc.expectedTEID, tc.expectedSrc, tc.expectedDst)
		})
	}
}

// TestCalculateIPv4Checksum tests the IPv4 checksum calculation
func TestCalculateIPv4Checksum(t *testing.T) {
	// Create a test IPv4 header with checksum field zeroed
	header := []byte{
		0x45, 0x00, 0x00, 0x3C, // Version, IHL, DSCP, Total Length
		0x1C, 0x46, 0x40, 0x00, // Identification, Flags, Fragment Offset
		0x40, 0x06, 0x00, 0x00, // TTL, Protocol, Checksum (zero for calculation)
		0xAC, 0x10, 0x0A, 0x63, // Source IP: 172.16.10.99
		0xAC, 0x10, 0x0A, 0x0C, // Dest IP: 172.16.10.12
	}

	checksum := calculateIPv4Checksum(header)

	// Verify checksum is calculated (non-zero)
	if checksum == 0 {
		t.Error("Checksum should not be zero")
	}

	// Now set the checksum in the header
	header[10] = byte(checksum >> 8)
	header[11] = byte(checksum & 0xFF)

	// Recalculate with checksum zeroed again to verify
	header[10] = 0
	header[11] = 0
	verifyChecksum := calculateIPv4Checksum(header)

	if verifyChecksum != checksum {
		t.Errorf("Checksum mismatch: first calculation 0x%04X, second 0x%04X", checksum, verifyChecksum)
	}
}

// Helper function to verify end marker packet structure
func verifyEndMarkerPacket(t *testing.T, packet []byte, expectedTEID uint32, expectedSrcIP, expectedDstIP string) {
	t.Helper()

	// Verify packet size
	if len(packet) != endMarkerSize {
		t.Errorf("expected packet size %d, got %d", endMarkerSize, len(packet))
		return
	}

	// Verify Ethernet header
	if binary.BigEndian.Uint16(packet[ethTypeOffset:]) != 0x0800 {
		t.Error("Ethernet type should be 0x0800 (IPv4)")
	}

	// Verify IPv4 header
	if packet[ipOffset] != 0x45 {
		t.Errorf("IPv4 version/IHL should be 0x45, got 0x%02X", packet[ipOffset])
	}
	if packet[ipOffset+9] != 17 {
		t.Errorf("IPv4 protocol should be 17 (UDP), got %d", packet[ipOffset+9])
	}

	// Verify source and destination IPs
	srcIP := net.IPv4(packet[ipSrcOffset], packet[ipSrcOffset+1],
		packet[ipSrcOffset+2], packet[ipSrcOffset+3])
	dstIP := net.IPv4(packet[ipDstOffset], packet[ipDstOffset+1],
		packet[ipDstOffset+2], packet[ipDstOffset+3])

	if srcIP.String() != expectedSrcIP {
		t.Errorf("expected source IP %s, got %s", expectedSrcIP, srcIP.String())
	}
	if dstIP.String() != expectedDstIP {
		t.Errorf("expected dest IP %s, got %s", expectedDstIP, dstIP.String())
	}

	// Verify UDP header
	srcPort := binary.BigEndian.Uint16(packet[udpOffset:])
	dstPort := binary.BigEndian.Uint16(packet[udpOffset+2:])
	if srcPort != tunnelGTPUPort || dstPort != tunnelGTPUPort {
		t.Errorf("expected UDP ports %d/%d, got %d/%d", tunnelGTPUPort, tunnelGTPUPort, srcPort, dstPort)
	}

	// Verify GTP-U header
	if packet[gtpOffset] != 0x30 {
		t.Errorf("GTP version/flags should be 0x30, got 0x%02X", packet[gtpOffset])
	}
	if packet[gtpOffset+1] != gtpEndMarkerType {
		t.Errorf("GTP message type should be %d (End Marker), got %d",
			gtpEndMarkerType, packet[gtpOffset+1])
	}

	msgLen := binary.BigEndian.Uint16(packet[gtpOffset+2:])
	if msgLen != 0 {
		t.Errorf("GTP message length should be 0, got %d", msgLen)
	}

	teid := binary.BigEndian.Uint32(packet[gtpOffset+4:])
	if teid != expectedTEID {
		t.Errorf("expected TEID 0x%08X, got 0x%08X", expectedTEID, teid)
	}

	// Verify IPv4 checksum is valid
	// When checksum is correct, recalculating should give the complement
	checksumInPacket := binary.BigEndian.Uint16(packet[ipChecksumOffset:])
	if checksumInPacket == 0 {
		t.Error("IPv4 checksum should not be zero")
	}

	// Create a copy of the header and zero out checksum field for verification
	headerCopy := make([]byte, ipv4HeaderSize)
	copy(headerCopy, packet[ipOffset:ipOffset+ipv4HeaderSize])
	headerCopy[10] = 0
	headerCopy[11] = 0
	recalculated := calculateIPv4Checksum(headerCopy)

	if recalculated != checksumInPacket {
		t.Errorf("IPv4 checksum mismatch: expected 0x%04X, got 0x%04X", checksumInPacket, recalculated)
	}
}
