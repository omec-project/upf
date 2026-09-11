// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"net"
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// The IP pool is built once in NewUPF and belongs to the upf, so it outlives every
// PFCPConn. An address a connection does not return when it goes away is therefore lost
// for the life of the process -- the session is forgotten and its local SEID is never
// presented again -- and an association that comes back gets a smaller pool than it left.

type noopMetrics struct{}

func (noopMetrics) SaveMessages(*metrics.Message) {}
func (noopMetrics) SaveSessions(*metrics.Session) {}
func (noopMetrics) Stop() error                   { return nil }

// allocatingSession is what a UPF-allocated address looks like once parsed: a core PDR
// carrying allocIPFlag, which is the pair releaseAllocatedIPs looks for.
func allocatingSession(t *testing.T, pool *IPPool, seid uint64) PFCPSession {
	t.Helper()

	ip, err := pool.LookupOrAllocIP(seid)
	if err != nil {
		t.Fatalf("pool would not allocate for seid %d: %v", seid, err)
	}

	return PFCPSession{
		localSEID: seid,
		metrics:   &metrics.Session{},
		PacketForwardingRules: PacketForwardingRules{
			pdrs: []pdr{{
				srcIface:    core,
				allocIPFlag: true,
				ueAddress:   ip2int(ip),
			}},
		},
	}
}

// teardownConn is an association holding one session, with a pool of exactly two
// addresses -- capacity is what distinguishes released from leaked, because DeallocIP
// appends to the back of the free queue while LookupOrAllocIP takes from the front.
func teardownConn(t *testing.T, pool *IPPool, sessions ...PFCPSession) *PFCPConn {
	t.Helper()

	// A dialled socket, not a listening one: executeShutdown reads RemoteAddr, which is
	// nil until the conn has a peer.
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { l.Close() }) //nolint:errcheck // test cleanup

	c, err := (&net.Dialer{}).DialContext(context.Background(), "udp", l.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { c.Close() }) //nolint:errcheck // test cleanup

	store := NewInMemoryStore()
	for _, s := range sessions {
		if err := store.PutSession(s); err != nil {
			t.Fatalf("could not seed the store: %v", err)
		}
	}

	return &PFCPConn{
		Conn:           c,
		store:          store,
		upf:            &upf{datapath: &fakeDP{}, ippool: pool},
		shutdown:       make(chan struct{}),
		InstrumentPFCP: noopMetrics{},
	}
}

func TestShutdownReleasesTheAddressesItsSessionsHold(t *testing.T) {
	pool, err := NewIPPool("10.251.0.0/30") // .1 and .2
	if err != nil {
		t.Fatal(err)
	}

	pConn := teardownConn(t, pool, allocatingSession(t, pool, 1))

	pConn.executeShutdown()

	// Two more must fit. Without the release the shut-down session still holds one of
	// the two, and the pool has no way to ever get it back.
	for i := uint64(2); i <= 3; i++ {
		if _, err := pool.LookupOrAllocIP(i); err != nil {
			t.Fatalf("allocation %d of 2 after a shutdown failed: %v; "+
				"the connection did not release the address its session held", i-1, err)
		}
	}
}

// The same loss, one session at a time: the peer answers a Session Report with "context
// not found", so the session is deleted locally -- and its address has to go with it.
func TestLocalDeleteOnContextNotFoundReleasesTheAddress(t *testing.T) {
	pool, err := NewIPPool("10.251.0.0/30")
	if err != nil {
		t.Fatal(err)
	}

	const seid = uint64(1)

	pConn := teardownConn(t, pool, allocatingSession(t, pool, seid))

	srres := message.NewSessionReportResponse(0, 0, seid, 1, 0,
		ie.NewCause(ie.CauseSessionContextNotFound))

	if err := pConn.handleSessionReportResponse(srres); err != nil {
		t.Fatalf("handling a context-not-found report response: %v", err)
	}

	for i := uint64(2); i <= 3; i++ {
		if _, err := pool.LookupOrAllocIP(i); err != nil {
			t.Fatalf("allocation %d of 2 after a local delete failed: %v; "+
				"the session deleted locally did not release its address", i-1, err)
		}
	}
}
