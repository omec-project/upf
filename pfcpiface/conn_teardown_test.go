// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
// carrying allocIPFlag, plus the pool inventory entry the release is keyed by.
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
// addresses -- capacity is what distinguishes released from leaked, because a release
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

	const (
		localSEID  = uint64(1)
		remoteSEID = uint64(0xC0FFEE)
	)

	// Distinct SEIDs, so the test says which of the two the handler has to look up.
	sess := allocatingSession(t, pool, localSEID)
	sess.remoteSEID = remoteSEID

	pConn := teardownConn(t, pool, sess)

	// A PFCP message carries the SEID allocated by whoever receives it, so a response
	// from the control plane carries the UPF's local one -- the same convention this
	// file already reads an incoming Modification or Deletion Request by, and the
	// opposite of the Session Report Request that provoked it, which is addressed with
	// remoteSEID. The store is keyed by localSEID and holds no remote-to-local mapping,
	// so nothing else would resolve either.
	srres := message.NewSessionReportResponse(0, 0, localSEID, 1, 0,
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

// scriptedTeardownDP answers each session's removal batches with the next completion in
// its script, keyed by the UE address of the session's PDR -- accepted either way, which is
// how bess answers a batch that ran out of time -- and records how many batches each
// session was sent. A session with no script finishes. The retry sends its batches
// concurrently, so the fake is locked.
type scriptedTeardownDP struct {
	fakeDP

	mu       sync.Mutex
	finishes map[uint32][]bool
	batches  map[uint32]int
}

func (d *scriptedTeardownDP) SendMsgToUPF(method upfMsgType, all, newRules PacketForwardingRules) uint8 {
	cause, _ := d.SendMsgToUPFWithCompletion(method, all, newRules)
	return cause
}

func (d *scriptedTeardownDP) SendMsgToUPFWithCompletion(
	_ upfMsgType, all, _ PacketForwardingRules,
) (uint8, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.batches == nil {
		d.batches = map[uint32]int{}
	}

	ue := all.pdrs[0].ueAddress
	n := d.batches[ue]
	d.batches[ue]++

	finished := true
	if script := d.finishes[ue]; n < len(script) {
		finished = script[n]
	}

	return ie.CauseRequestAccepted, finished
}

// teardownOfTwo is an association holding two sessions with UPF-allocated addresses, from a
// pool of exactly those two, so capacity says which came back.
func teardownOfTwo(t *testing.T) (*PFCPConn, *IPPool, [2]PFCPSession) {
	t.Helper()

	pool, err := NewIPPool("10.251.0.0/30") // .1 and .2
	if err != nil {
		t.Fatal(err)
	}

	sessions := [2]PFCPSession{allocatingSession(t, pool, 1), allocatingSession(t, pool, 2)}

	return teardownConn(t, pool, sessions[0], sessions[1]), pool, sessions
}

// TestATeardownRemovalThatDidNotFinishIsRetried: both sessions' removals run out of time,
// so each is retried once. The retries finish, as a late removal would, so both addresses
// come back.
func TestATeardownRemovalThatDidNotFinishIsRetried(t *testing.T) {
	pConn, pool, sessions := teardownOfTwo(t)

	a, b := sessions[0].pdrs[0].ueAddress, sessions[1].pdrs[0].ueAddress
	dp := &scriptedTeardownDP{finishes: map[uint32][]bool{a: {false, true}, b: {false, true}}}
	pConn.upf.datapath = dp

	pConn.executeShutdown()

	if dp.batches[a] != 2 || dp.batches[b] != 2 {
		t.Fatalf("the teardown sent the sessions %d and %d removal batches, expected a "+
			"removal and one retry each", dp.batches[a], dp.batches[b])
	}

	for i := uint64(3); i <= 4; i++ {
		if _, err := pool.LookupOrAllocIP(i); err != nil {
			t.Fatalf("allocation %d of 2 after the retries finished failed: %v; a session "+
				"whose rules were removed on the retry did not give its address back", i-2, err)
		}
	}
}

// TestATeardownRemovalThatNeverFinishesHoldsOnlyItsOwnAddress: one session's retry runs out
// of time too, so its rules may still be in the datapath with nothing left that could
// remove them, and its address stays held rather than go to a UE a surviving rule would
// match. The other session's retry finished, and its address must not be held with it.
func TestATeardownRemovalThatNeverFinishesHoldsOnlyItsOwnAddress(t *testing.T) {
	pConn, pool, sessions := teardownOfTwo(t)

	stuck, drained := sessions[0].pdrs[0].ueAddress, sessions[1].pdrs[0].ueAddress
	dp := &scriptedTeardownDP{finishes: map[uint32][]bool{
		stuck:   {false, false},
		drained: {false, true},
	}}
	pConn.upf.datapath = dp

	pConn.executeShutdown()

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("%d session(s) still stored after the teardown", n)
	}

	got, err := pool.LookupOrAllocIP(3)
	if err != nil {
		t.Fatalf("the pool has no free address at all: %v; the session whose retry "+
			"finished was held with the one whose retry did not", err)
	}

	if ip2int(got) != drained {
		t.Fatalf("the pool gave out %s, expected %s, the address of the session whose "+
			"rules were removed", got, int2ip(drained))
	}

	if _, err := pool.LookupOrAllocIP(4); err == nil {
		t.Fatal("the address of a session whose removal never finished went back to the " +
			"pool, while a rule it may still have in the datapath matches it")
	}
}

// rendezvousTeardownDP lets a session's first removal run out of time and holds each retry
// until every session's retry has arrived, so a retry that runs sessions one after another
// never gets past the first.
type rendezvousTeardownDP struct {
	fakeDP

	mu      sync.Mutex
	seen    map[uint32]int
	arrived sync.WaitGroup
	apart   atomic.Bool
}

func (d *rendezvousTeardownDP) SendMsgToUPFWithCompletion(
	_ upfMsgType, all, _ PacketForwardingRules,
) (uint8, bool) {
	d.mu.Lock()
	ue := all.pdrs[0].ueAddress
	d.seen[ue]++
	first := d.seen[ue] == 1
	d.mu.Unlock()

	if first {
		return ie.CauseRequestAccepted, false
	}

	d.arrived.Done()

	done := make(chan struct{})

	go func() {
		d.arrived.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		d.apart.Store(true)
	}

	return ie.CauseRequestAccepted, true
}

// TestTeardownRetriesRunTogether: the retry costs one batch's timeout rather than one per
// session only if the sessions' retries are in flight at the same time.
func TestTeardownRetriesRunTogether(t *testing.T) {
	pConn, _, _ := teardownOfTwo(t)

	dp := &rendezvousTeardownDP{seen: map[uint32]int{}}
	dp.arrived.Add(2)
	pConn.upf.datapath = dp

	pConn.executeShutdown()

	if dp.apart.Load() {
		t.Fatal("a retry waited for the other session's and it never came: the retries ran " +
			"one after another, and each adds a batch's timeout to the teardown")
	}
}
