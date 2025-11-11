// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package pfcpiface

import (
	"net"
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
