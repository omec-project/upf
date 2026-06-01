// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package pfcpiface

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// fakeDP implements the datapath interface with no-op methods for testing.
type fakeDP struct{}

func (f *fakeDP) Exit()                                        {}
func (f *fakeDP) SetUpfInfo(u *upf, conf *Conf)                {}
func (f *fakeDP) AddSliceInfo(sliceInfo *SliceInfo) error      { return nil }
func (f *fakeDP) SendEndMarkers(endMarkerList *[][]byte) error { return nil }
func (f *fakeDP) SendMsgToUPF(method upfMsgType, all PacketForwardingRules, newRules PacketForwardingRules) uint8 {
	return 1
}
func (f *fakeDP) IsConnected(accessIP *net.IP) bool                                     { return true }
func (f *fakeDP) SummaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric)    {}
func (f *fakeDP) PortStats(uc *upfCollector, ch chan<- prometheus.Metric)               {}
func (f *fakeDP) SummaryGtpuLatency(uc *upfCollector, ch chan<- prometheus.Metric)      {}
func (f *fakeDP) SessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) error { return nil }

// Test that a truncated (simulated unexpected EOF) Association Setup Request
// is handled without causing a panic in the PFCP message handler.
func TestHandlePFCPMsg_TruncatedAssociationRequest_NoPanic(t *testing.T) {
	// Build a valid Association Setup Request
	seq := uint32(1)
	asreq := message.NewAssociationSetupRequest(seq,
		ie.NewNodeID("", "", "test-smf"),
		ie.NewRecoveryTimeStamp(time.Now()),
	)

	out := make([]byte, asreq.MarshalLen())
	if err := asreq.MarshalTo(out); err != nil {
		t.Fatalf("failed to marshal association request: %v", err)
	}

	// Truncate the buffer to simulate unexpected EOF / truncated packet
	if len(out) < 6 {
		t.Fatalf("unexpected small PFCP message")
	}
	trunc := out[:len(out)-5]

	// Create a UDP socket to satisfy RemoteAddr usage
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to create udp socket: %v", err)
	}
	defer l.Close()

	// Prepare PFCPConn with fake datapath that reports connected
	p := &PFCPConn{
		Conn: l,
		upf: &upf{
			datapath: &fakeDP{},
		},
	}

	// Call handler; should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked on truncated message: %v", r)
		}
	}()

	p.HandlePFCPMsg(trunc)
}

func TestDumpRawPFCP_DoesNotOverwriteExistingFile(t *testing.T) {
	oldRandRead := dumpRawPFCPRandRead
	oldNow := dumpRawPFCPNow
	t.Cleanup(func() {
		dumpRawPFCPRandRead = oldRandRead
		dumpRawPFCPNow = oldNow
	})

	fixedTime := time.Date(2026, time.May, 29, 12, 34, 56, 0, time.UTC)
	dumpRawPFCPNow = func() time.Time { return fixedTime }
	dumpRawPFCPRandRead = func(b []byte) (int, error) {
		copy(b, []byte{0xaa, 0xbb, 0xcc, 0xdd})
		return len(b), nil
	}

	dumpDir := t.TempDir()
	addr := "127.0.0.1:8805"
	firstPayload := []byte("first payload")
	secondPayload := []byte("second payload")

	if err := dumpRawPFCP(dumpDir, addr, firstPayload); err != nil {
		t.Fatalf("first dumpRawPFCP failed: %v", err)
	}

	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("failed to read dump dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 dump file, got %d", len(entries))
	}

	dumpPath := filepath.Join(dumpDir, entries[0].Name())
	if err := dumpRawPFCP(dumpDir, addr, secondPayload); err == nil {
		t.Fatal("expected second dumpRawPFCP call to fail on existing file")
	}

	got, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("failed to read dump file: %v", err)
	}
	if !bytes.Equal(got, firstPayload) {
		t.Fatalf("dump file was overwritten: got %q want %q", string(got), string(firstPayload))
	}

	entries, err = os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("failed to reread dump dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected dump dir to still contain 1 file, got %d", len(entries))
	}
}
