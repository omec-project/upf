// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// Session handlers run on the association's reader goroutine and teardown runs on
// another -- the heartbeat monitor's, say. A handler that teardown's snapshot of the
// store misses is never reclaimed, and one that removes a session the snapshot also
// holds removes it twice. Teardown therefore takes its snapshot only while no session
// handler is running, and a handler that starts after it refuses the message.

// gatedDP blocks the first write of one method until released, so a test can start a
// teardown while a handler is in the middle of its datapath write. Every write finishes
// and is accepted.
type gatedDP struct {
	fakeDP

	gate     upfMsgType
	entered  chan struct{}
	release  chan struct{}
	mu       sync.Mutex
	gated    bool
	numCalls int
}

func newGatedDP(gate upfMsgType) *gatedDP {
	return &gatedDP{gate: gate, entered: make(chan struct{}), release: make(chan struct{})}
}

func (d *gatedDP) SendMsgToUPF(method upfMsgType, all, newRules PacketForwardingRules) uint8 {
	cause, _ := d.SendMsgToUPFWithCompletion(method, all, newRules)
	return cause
}

func (d *gatedDP) SendMsgToUPFWithCompletion(method upfMsgType, _, _ PacketForwardingRules) (uint8, bool) {
	d.mu.Lock()
	d.numCalls++
	block := method == d.gate && !d.gated
	if block {
		d.gated = true
	}
	d.mu.Unlock()

	if block {
		close(d.entered)
		<-d.release
	}

	return ie.CauseRequestAccepted, true
}

func (d *gatedDP) calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.numCalls
}

func allocatingEstablishment(remoteSEID uint64) *message.SessionEstablishmentRequest {
	return message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(remoteSEID, net.ParseIP("127.0.0.1"), nil),
		ie.NewCreatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(0),
			ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceCore), chooseUEAddress()),
			ie.NewFARID(downlinkFARID),
		),
		downlinkFAR(),
	)
}

// raceTeardown starts a teardown while the gated write is blocked, gives it a moment to
// finish without waiting for the handler -- which it must not be able to do -- and then
// lets the handler go and waits for both.
func raceTeardown(t *testing.T, pConn *PFCPConn, dp *gatedDP, handler func()) {
	t.Helper()

	handled := make(chan struct{})

	go func() {
		defer close(handled)
		handler()
	}()

	select {
	case <-dp.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler never reached the gated datapath write")
	}

	tornDown := make(chan struct{})

	go func() {
		defer close(tornDown)
		pConn.Shutdown()
	}()

	select {
	case <-tornDown:
	case <-time.After(200 * time.Millisecond):
	}

	close(dp.release)

	for _, done := range []chan struct{}{handled, tornDown} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the handler or the teardown did not finish")
		}
	}
}

// TestAnEstablishmentInFlightWhenTeardownStartsIsReclaimed: the establishment is in its
// datapath write when teardown starts. Teardown must not take its snapshot until the
// handler has stored the session, or that session -- its rules, its gauge entry, its
// address -- is never reclaimed.
func TestAnEstablishmentInFlightWhenTeardownStartsIsReclaimed(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	counter := &countingMetrics{}
	pConn.InstrumentPFCP = counter
	dp := newGatedDP(upfMsgTypeAdd)
	pConn.upf.datapath = dp

	var handlerErr error

	raceTeardown(t, pConn, dp, func() {
		_, handlerErr = pConn.handleSessionEstablishmentRequest(allocatingEstablishment(testRemoteSEID))
	})

	if handlerErr != nil {
		t.Fatalf("the establishment was refused: %v; it was in flight before the teardown "+
			"began, so it should have completed", handlerErr)
	}

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("%d session(s) stored after the teardown; the snapshot missed the "+
			"establishment in flight, and nothing will reclaim it", n)
	}

	if counter.live != 0 {
		t.Fatalf("%d session(s) still counted as live after the teardown", counter.live)
	}

	assertPoolIsWhole(t, pool, "a teardown racing an establishment")
}

// TestADeletionRacingTeardownCountsTheSessionOutOnce: the deletion is in its datapath
// write when teardown starts. If the snapshot still holds the session, both remove it,
// and the gauge counts it out twice.
func TestADeletionRacingTeardownCountsTheSessionOutOnce(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	counter := &countingMetrics{}
	pConn.InstrumentPFCP = counter
	localSEID := establishAllocatingSession(t, pConn, testRemoteSEID)

	dp := newGatedDP(upfMsgTypeDel)
	pConn.upf.datapath = dp

	var handlerErr error

	raceTeardown(t, pConn, dp, func() {
		_, handlerErr = pConn.handleSessionDeletionRequest(
			message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	})

	if handlerErr != nil {
		t.Fatalf("the deletion was refused: %v; it was in flight before the teardown began, "+
			"so it should have completed", handlerErr)
	}

	if counter.live != 0 {
		t.Fatalf("the gauge counts %d live session(s) after a deletion and a teardown of "+
			"one session; it was removed twice", counter.live)
	}

	assertPoolIsWhole(t, pool, "a deletion racing a teardown")
}

func causeOf(t *testing.T, causeIE *ie.IE) uint8 {
	t.Helper()

	if causeIE == nil {
		t.Fatal("the response carries no Cause IE")
	}

	cause, err := causeIE.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause: %v", err)
	}

	return cause
}

// TestASessionMessageAfterTeardownBeganChangesNothing: once teardown has started, a
// session message is refused with "No established PFCP Association" before it touches
// the datapath, the gauge, the pool or the store -- teardown owns every session now.
func TestASessionMessageAfterTeardownBeganChangesNothing(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	counter := &countingMetrics{}
	pConn.InstrumentPFCP = counter
	const remoteSEID = 0xBEEF

	localSEID := establishAllocatingSession(t, pConn, remoteSEID)

	dp := newGatedDP(upfMsgTypeClear) // never gates: nothing here sends a clear
	pConn.upf.datapath = dp
	pConn.isShutdown.Store(true)

	assertReleasing := func(what string, err error) {
		t.Helper()

		// The establishment's refusals come wrapped in a HandlePFCPMsgError, which
		// does not unwrap.
		var wrapped *HandlePFCPMsgError
		if errors.As(err, &wrapped) {
			err = wrapped.Err
		}

		if !errors.Is(err, ErrAssocReleasing) {
			t.Errorf("the %s was refused with %v, expected %v", what, err, ErrAssocReleasing)
		}
	}

	rsp, err := pConn.handleSessionEstablishmentRequest(allocatingEstablishment(remoteSEID + 1))
	assertReleasing("establishment", err)

	if cause := causeOf(t, rsp.(*message.SessionEstablishmentResponse).Cause); cause != ie.CauseNoEstablishedPFCPAssociation {
		t.Errorf("the establishment was answered with cause %d, expected %d",
			cause, ie.CauseNoEstablishedPFCPAssociation)
	}

	rsp, err = pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
			ie.NewUpdateFAR(ie.NewFARID(downlinkFARID), ie.NewApplyAction(ActionDrop))))
	assertReleasing("modification", err)

	smres := rsp.(*message.SessionModificationResponse)
	if cause := causeOf(t, smres.Cause); cause != ie.CauseNoEstablishedPFCPAssociation {
		t.Errorf("the modification was answered with cause %d, expected %d",
			cause, ie.CauseNoEstablishedPFCPAssociation)
	}

	if smres.SEID() != remoteSEID {
		t.Errorf("the modification's refusal carries SEID %#x, expected the session's %#x",
			smres.SEID(), uint64(remoteSEID))
	}

	rsp, err = pConn.handleSessionDeletionRequest(message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	assertReleasing("deletion", err)

	if cause := causeOf(t, rsp.(*message.SessionDeletionResponse).Cause); cause != ie.CauseNoEstablishedPFCPAssociation {
		t.Errorf("the deletion was answered with cause %d, expected %d",
			cause, ie.CauseNoEstablishedPFCPAssociation)
	}

	// Teardown removes sessions after releasing the lock, so a message queued behind its
	// snapshot can find its session already gone. It is still refused as the association
	// going away, not answered as a session the node never had. A SEID the store does not
	// hold stands in for one teardown has just removed.
	rsp, err = pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, localSEID+1, 5, 0))
	assertReleasing("modification of a session teardown has removed", err)

	if cause := causeOf(t, rsp.(*message.SessionModificationResponse).Cause); cause != ie.CauseNoEstablishedPFCPAssociation {
		t.Errorf("a modification of a removed session was answered with cause %d, expected %d",
			cause, ie.CauseNoEstablishedPFCPAssociation)
	}

	rsp, err = pConn.handleSessionDeletionRequest(message.NewSessionDeletionRequest(0, 0, localSEID+1, 6, 0))
	assertReleasing("deletion of a session teardown has removed", err)

	if cause := causeOf(t, rsp.(*message.SessionDeletionResponse).Cause); cause != ie.CauseNoEstablishedPFCPAssociation {
		t.Errorf("a deletion of a removed session was answered with cause %d, expected %d",
			cause, ie.CauseNoEstablishedPFCPAssociation)
	}

	if err := pConn.handleSessionReportResponse(message.NewSessionReportResponse(0, 0, localSEID+1, 7, 0,
		ie.NewCause(ie.CauseSessionContextNotFound))); err != nil {
		t.Errorf("a report response for a removed session was answered with %v, expected nothing", err)
	}

	if err := pConn.handleSessionReportResponse(message.NewSessionReportResponse(0, 0, localSEID, 4, 0,
		ie.NewCause(ie.CauseSessionContextNotFound))); err != nil {
		t.Errorf("the report response was answered with %v, expected nothing", err)
	}

	if n := dp.calls(); n != 0 {
		t.Errorf("the refused messages made %d datapath call(s), expected none", n)
	}

	if counter.live != 1 {
		t.Errorf("%d session(s) counted as live, expected the one session, untouched", counter.live)
	}

	if _, held := pConn.store.GetSession(localSEID); !held {
		t.Error("a refused message removed the session teardown is about to reclaim")
	}

	// The session holds one address of the two; nothing gave it back.
	if _, err := pool.LookupOrAllocIP(0xF001); err != nil {
		t.Fatalf("the pool has no free address: %v", err)
	}

	if _, err := pool.LookupOrAllocIP(0xF002); err == nil {
		t.Error("a refused message released the session's address")
	}
}

// endMarkerDP records whether the association's session lock was free when the handler
// sent its end markers.
type endMarkerDP struct {
	fakeDP

	conn     *PFCPConn
	sent     bool
	lockFree bool
}

func (d *endMarkerDP) SendEndMarkers(*[][]byte) error {
	d.sent = true

	if d.conn.sessionsMu.TryLock() {
		d.lockFree = true
		d.conn.sessionsMu.Unlock()
	}

	return nil
}

// TestAModificationSendsItsEndMarkersOutsideTheLock: sending an end marker can block --
// bess queues them on a bounded channel whose drain loop may not be running -- and
// teardown waits for the lock, so a send that blocks while holding it would stall the
// association's teardown and node shutdown behind it.
func TestAModificationSendsItsEndMarkersOutsideTheLock(t *testing.T) {
	pConn, _, localSEID := rollbackConn(t)
	pConn.upf.enableEndMarker = true

	dp := &endMarkerDP{conn: pConn}
	pConn.upf.datapath = dp

	modifySession(t, pConn, localSEID, ie.NewUpdateFAR(
		ie.NewFARID(downlinkFARID),
		ie.NewApplyAction(ActionForward),
		ie.NewUpdateForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewPFCPSMReqFlags(0x02), // SNDEM
		),
	))

	if !dp.sent {
		t.Fatal("the modification sent no end markers, so this test proves nothing")
	}

	if !dp.lockFree {
		t.Fatal("the end markers were sent while the session lock was held; a send that " +
			"blocks would hold up the association's teardown")
	}
}
