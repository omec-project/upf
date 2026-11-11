// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// A control plane that lost a Session Deletion Response retransmits the request, which
// must be answered with the response to the first (TS 29.244 clause 6.4): the session is
// gone by then, and "Session context not found" would tell the control plane its
// deletion failed.
func TestARetransmittedDeletionIsAnsweredWithTheFirstResponse(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())
	otherSEID := establishStaticSession(t, pConn,
		corePDR(secondPDRID, secondUEAddress), downlinkFAR())

	first, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err != nil {
		t.Fatalf("deletion refused: %v", err)
	}

	again, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err != nil {
		t.Fatalf("the retransmitted deletion was refused: %v", err)
	}

	if cause := deletionCause(t, again); cause != ie.CauseRequestAccepted || again != first {
		t.Fatalf("the retransmitted deletion was answered with cause %d, not with the first response", cause)
	}

	if again.Sequence() != 3 {
		t.Fatalf("the retransmission was answered with sequence number %d, expected 3", again.Sequence())
	}

	assertDeletionSEID(t, again, testRemoteSEID)

	// Another request for the session is not a retransmission.
	other, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 4, 0))
	if err == nil {
		t.Fatal("a new deletion of a deleted session was answered as done")
	}

	if cause := deletionCause(t, other); cause != ie.CauseSessionContextNotFound {
		t.Fatalf("a new deletion of a deleted session was answered with cause %d, expected %d",
			cause, ie.CauseSessionContextNotFound)
	}

	// Nor is a request for another session with the same sequence number.
	if _, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, otherSEID, 3, 0)); err != nil {
		t.Fatalf("the deletion of another session was refused: %v", err)
	}

	if _, held := pConn.store.GetSession(otherSEID); held {
		t.Fatal("a deletion of another session with the same sequence number was answered " +
			"from the cache, and the session was not deleted")
	}
}

// A refused deletion leaves the session, so its retransmission is handled afresh rather
// than answered with the refusal.
func TestARefusedDeletionIsNotReplayed(t *testing.T) {
	pConn, _, localSEID := deletionConn(t)

	if _, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0)); err == nil {
		t.Fatal("a deletion the datapath did not finish was answered as done")
	}

	pConn.upf.datapath = &fakeDP{} // the datapath recovers

	rsp, err := pConn.handleSessionDeletionRequest(
		message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0))
	if err != nil {
		t.Fatalf("the retransmitted deletion was refused: %v", err)
	}

	if cause := deletionCause(t, rsp); cause != ie.CauseRequestAccepted {
		t.Fatalf("the retransmitted deletion was answered with cause %d, expected it handled afresh "+
			"and accepted", cause)
	}

	if _, held := pConn.store.GetSession(localSEID); held {
		t.Fatal("the session is still stored after the retransmitted deletion")
	}
}

func TestADeletionResponseIsKeptForTheRetentionPeriod(t *testing.T) {
	var c deletionResponses

	start := time.Now()
	first := deletionKey{localSEID: 1, seq: 1}
	rsp := message.NewSessionDeletionResponse(0, 0, 0, 1, 0)

	c.put(first, rsp, start)

	// However many deletions follow, as many as a mass release brings.
	for seq := range uint32(10000) {
		c.put(deletionKey{localSEID: 2, seq: seq}, rsp, start.Add(time.Second))
	}

	if _, ok := c.get(first, start.Add(deletionResponseRetention-time.Millisecond)); !ok {
		t.Fatal("a response is dropped before the retention period ends")
	}

	if _, ok := c.get(first, start.Add(deletionResponseRetention)); ok {
		t.Fatal("a response is kept past the retention period")
	}

	c.put(deletionKey{localSEID: 3, seq: 1}, rsp, start.Add(deletionResponseRetention+time.Second))

	if len(c.byKey) != 1 || len(c.order) != 1 {
		t.Fatalf("%d responses are kept past the retention period, expected only the last one",
			len(c.byKey)-1)
	}
}
