// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// notifyConn is an association with its own session store, which is what makes a
// notification's owner ambiguous when more than one exists.
func notifyConn(t *testing.T, sessions SessionsStore) *PFCPConn {
	t.Helper()

	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { l.Close() }) //nolint:errcheck // test cleanup

	return &PFCPConn{Conn: l, store: sessions, upf: &upf{datapath: &fakeDP{}}}
}

// A downlink data notification carries only an F-SEID, so the node has to offer it to
// each association until one recognises it. Reporting ownership is what makes that
// possible: the loop used to stop at the first association the map yielded, and if that
// one did not hold the session the notification was dropped and no Session Report was
// sent -- an idle UE left unreachable by which peer happened to be enumerated first.
func TestHandleDigestReportReportsWhetherItOwnsTheSession(t *testing.T) {
	const held = uint64(0x1234)

	owner := NewInMemoryStore()
	if err := owner.PutSession(PFCPSession{localSEID: held, remoteSEID: 0x4321}); err != nil {
		t.Fatalf("store the session: %v", err)
	}

	stranger := notifyConn(t, NewInMemoryStore())
	if stranger.handleDigestReport(held) {
		t.Error("an association that does not hold the session claimed the notification")
	}

	// The owner has no PDR for the downlink, so no report is built -- but it must still
	// claim the notification, or the node would go on offering it to peers that do not
	// know the F-SEID at all.
	if !notifyConn(t, owner).handleDigestReport(held) {
		t.Error("the association holding the session did not claim the notification")
	}
}

// The control plane's only lever for "stop buffering" is a flag this datapath cannot act
// on, so the one thing it can do is say so. Guarding mainly that reading the flags of a
// response that has none does not panic, and that the bit is the one TS 29.244 names.
func TestReportDropBufferedRequest(t *testing.T) {
	tests := []struct {
		name string
		rsp  *message.SessionReportResponse
	}{
		{
			name: "no flags IE at all",
			rsp:  message.NewSessionReportResponse(0, 0, 1, 1, 0, ie.NewCause(ie.CauseRequestRejected)),
		},
		{
			name: "flags without DROBU",
			rsp: message.NewSessionReportResponse(0, 0, 1, 1, 0,
				ie.NewCause(ie.CauseRequestRejected), ie.NewPFCPSRRspFlags(0x00)),
		},
		{
			name: "DROBU requested",
			rsp: message.NewSessionReportResponse(0, 0, 1, 1, 0,
				ie.NewCause(ie.CauseRequestRejected), ie.NewPFCPSRRspFlags(pfcpSRRspFlagDROBU)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reportDropBufferedRequest(tc.rsp)
		})
	}
}
