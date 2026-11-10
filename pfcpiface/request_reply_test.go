// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// requestConn is an association whose requests get no answer from their peer, and are
// given up after one short wait.
func requestConn(t *testing.T) *PFCPConn {
	t.Helper()

	pool, err := NewIPPool("10.251.0.0/30")
	if err != nil {
		t.Fatal(err)
	}

	pConn := teardownConn(t, pool)
	pConn.upf.maxReqRetries = 0
	pConn.upf.respTimeout = 10 * time.Millisecond

	return pConn
}

// deliverHeartbeatResponse hands the association a Heartbeat Response, as its reader
// would, and fails the test if the reader would block on it.
func deliverHeartbeatResponse(t *testing.T, pConn *PFCPConn, seq uint32) {
	t.Helper()

	rsp := message.NewHeartbeatResponse(seq, ie.NewRecoveryTimeStamp(time.Now()))

	buf := make([]byte, rsp.MarshalLen())
	if err := rsp.MarshalTo(buf); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() {
		pConn.HandlePFCPMsg(buf)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a response nobody waits for blocked the association's reader")
	}
}

// A request given up is no longer awaited, and a response that comes for it later is
// dropped.
func TestARequestThatTimedOutIsNoLongerAwaited(t *testing.T) {
	pConn := requestConn(t)

	r := pConn.getHeartBeatRequest()
	seq := r.msg.Sequence()

	if _, timeout := pConn.sendPFCPRequestMessage(r); !timeout {
		t.Fatal("expected the unanswered heartbeat to time out")
	}

	if _, ok := pConn.pendingReqs.Load(seq); ok {
		t.Fatal("the heartbeat that timed out is still awaited")
	}

	deliverHeartbeatResponse(t, pConn, seq)
}

// A response can arrive after its sender gave up but before it stopped awaiting one.
// The association's reader delivers it, and must not block on a sender that is gone:
// it would read nothing more for good.
func TestAResponseWhoseSenderGaveUpDoesNotBlockTheReader(t *testing.T) {
	pConn := requestConn(t)

	r := pConn.getHeartBeatRequest()
	pConn.pendingReqs.Store(r.msg.Sequence(), r) // awaited, but no sender is waiting

	deliverHeartbeatResponse(t, pConn, r.msg.Sequence())

	if _, ok := pConn.pendingReqs.Load(r.msg.Sequence()); ok {
		t.Fatal("the delivered response left its request registered")
	}
}

// An answered request returns its response, and is no longer awaited: a duplicate of the
// response, as a peer answering a retransmission sends, is dropped.
func TestAnAnsweredRequestIsNoLongerAwaited(t *testing.T) {
	pConn := requestConn(t)
	pConn.upf.respTimeout = 5 * time.Second

	r := pConn.getHeartBeatRequest()
	seq := r.msg.Sequence()

	type result struct {
		reply   message.Message
		timeout bool
	}

	done := make(chan result, 1)

	go func() {
		reply, timeout := pConn.sendPFCPRequestMessage(r)
		done <- result{reply, timeout}
	}()

	// The request is registered before it is sent; wait for it.
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, ok := pConn.pendingReqs.Load(seq); ok {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the request was never registered")
		}

		time.Sleep(time.Millisecond)
	}

	deliverHeartbeatResponse(t, pConn, seq)

	select {
	case got := <-done:
		if got.timeout || got.reply == nil || got.reply.Sequence() != seq {
			t.Fatalf("expected the response to sequence %d, got timeout=%v reply=%v", seq, got.timeout, got.reply)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the answered request did not return")
	}

	if _, ok := pConn.pendingReqs.Load(seq); ok {
		t.Fatal("the answered request is still awaited")
	}

	deliverHeartbeatResponse(t, pConn, seq) // the duplicate
}
