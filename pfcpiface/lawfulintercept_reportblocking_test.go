// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// unansweringADMF is an ADMF that accepts the connection and never answers, which is
// the condition a fault report is most likely to be issued under: the conditions worth
// reporting are the conditions under which the peer is most likely to be unreachable.
func unansweringADMF(t *testing.T) string {
	t.Helper()

	held := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-held:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(held)
		srv.Close()
	})

	return srv.URL
}

// TestTheReadLoopKeepsDrainingWhileAReportIsOutstanding is the data-plane half of
// "reporting a fault does not stall the path that observed it", and it is a timing
// claim, so it is driven rather than reasoned about.
//
// The read loop sits in front of an AF_UNIX SEQPACKET queue whose invariant its own
// comment states: everything done before returning to Read is time that queue spends
// filling, and what it overflows with is intercept product nobody can recover. A
// synchronous report on that path would make the remedy amplify the fault — the report
// that says copies were dropped before framing would drop the next several hundred —
// for as long as the ADMF took to answer, which is up to the client's full 10s timeout
// when it never answers at all.
//
// The framing side is deliberately absent, so the punt queue fills immediately and
// every subsequent datagram takes the x3FramingLost branch. With the report issued on
// the loop, the writes below block behind a stalled reader and this test times out.
func TestTheReadLoopKeepsDrainingWhileAReportIsOutstanding(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "li-x3.sock")

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Skipf("unixpacket sockets unavailable here: %v", err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup

	s := &liShipper{
		sockAddr: addr,
		senders:  make(map[string]x2x3.Sender),
		// One slot and nothing draining it: the second datagram onwards is a drop, so
		// the report fires on all but the first.
		punted:   []chan []byte{make(chan []byte, 1)},
		free:     make(chan []byte, 1),
		reporter: x1.NewReporter(unansweringADMF(t), "admfID", "neID", nil),
	}
	s.egressDown.Store(true)

	go s.shipLoop()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("the shipper never dialled the egress: %v", err)
	}
	defer conn.Close() //nolint:errcheck // test cleanup

	// Enough datagrams that a single 10s stall cannot be mistaken for slowness, and
	// small enough that a draining loop finishes them in milliseconds.
	const copies = 500

	payload := make([]byte, liTagLen+64)

	done := make(chan error, 1)
	go func() {
		for range copies {
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				done <- err

				return
			}
			if _, err := conn.Write(payload); err != nil {
				done <- err

				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the datapath could not hand the shipper its copies: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the read loop stopped draining the datapath socket while a report was outstanding")
	}
}

// slowFormReporter stands in for a reporter whose blocking form blocks — which is what
// x1.Reporter is, an mTLS round trip bounded only by its own 10s timeout. Notify sleeps;
// NotifyAsync returns at once. It also counts the calls, so a test can prove the callback
// under examination actually ran.
//
// A double rather than a real reporter and a hanging ADMF, because what is being asserted
// is which *form* this element's callback uses. Whether the blocking form really blocks is
// x1.Reporter's own property, and li asserts it there.
type slowFormReporter struct {
	block time.Duration

	mu    sync.Mutex
	sync  int
	async int
}

func (r *slowFormReporter) Notify(string, string) {
	r.mu.Lock()
	r.sync++
	r.mu.Unlock()
	time.Sleep(r.block)
}

func (r *slowFormReporter) NotifyAsync(issueType, description string) {
	r.mu.Lock()
	r.async++
	r.mu.Unlock()
}

func (r *slowFormReporter) counts() (syncCalls, asyncCalls int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.sync, r.async
}

// TestARefusedX1RequestIsAnsweredWithoutWaitingOnTheReporter is the
// provisioning-direction half of the rule the read-loop test covers for the data plane.
//
// OnAuthFailure runs synchronously on the X1 request goroutine and its own contract says
// it must not block. Reporting from it with the blocking form would hold the triggering
// interface open for as long as the ADMF took to answer — up to the reporter's full 10s
// timeout when it never answers at all. That would make this element's response time a
// function of whether its ADMF is up, measurable by whoever is probing the interface, on
// the path that refuses unauthorised tasking: precisely where an element should be
// quickest.
//
// **The callback is asserted to have run.** A refused request that never reached
// OnAuthFailure would pass the timing assertion against the blocking form too —
// measuring a path that does not report. OnPurge uses the same form
// for the same reason (a bulk deactivation reaches it on this goroutine) and is not driven
// separately, because producing a keepalive lapse means waiting one out.
func TestARefusedX1RequestIsAnsweredWithoutWaitingOnTheReporter(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)

	// A peer holding a certificate from the LI CA that binds it as a *network element*,
	// not as the ADMF. It authenticates at TLS and fails clause 8.2.4, which is the
	// condition OnAuthFailure exists for.
	wrongRoleCert, wrongRoleKey := liLeaf(t, dir, caCert, caKey, "NE", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}
	peerMat, err := mtls.Load(wrongRoleCert, wrongRoleKey, caPath)
	if err != nil {
		t.Fatalf("load peer material: %v", err)
	}

	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freePort(t)}

	reporter := &slowFormReporter{block: 10 * time.Second}
	if _, startErr := startTriggerListener(cfg, upfMat.ServerTLS(), reporter, nil,
		x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil); startErr != nil {
		t.Fatalf("startTriggerListener: %v", startErr)
	}

	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, peerMat.ClientTLS())

	start := time.Now()
	err = req.ActivateTask(x1.Trigger{
		XID:           types.XID(testXIDPrimary),
		ProductID:     types.XID(testXIDSecondary),
		CorrelationID: 7,
		SEID:          0x2632898145f4d191,
		// A destination, or the requester refuses this client-side and the request never
		// reaches the element at all, and the test would measure a validation error
		// instead of a refusal.
		DIDs: []string{testXIDTertiary},
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a peer whose certificate binds it as a network element rather than the ADMF " +
			"was allowed to task this element")
	}

	syncCalls, asyncCalls := reporter.counts()
	if syncCalls+asyncCalls == 0 {
		t.Fatalf("the authentication failure was not reported at all (err=%v), so this asserts "+
			"nothing about which form reports it", err)
	}
	if syncCalls > 0 {
		t.Errorf("the refusal was reported with the blocking form (%d calls): this element's answer "+
			"waits on its own reporting peer, which is observable to whoever is probing the interface",
			syncCalls)
	}
	if elapsed > 3*time.Second {
		t.Errorf("refusing an unauthorised request took %s while the reporter was unresponsive", elapsed)
	}
}

// TestASessionsCopiesReachOneFramingWorkerInReadOrder: a fragmented datagram is
// classified from its first fragment, and its later fragments follow that decision. If any
// worker could take any copy, a later fragment read after its first could be looked up
// before the worker holding the first had classified it, and be dropped as lost although
// it arrived in order. So each session's copies go to one worker's queue, in the order the
// read loop took them. The workers are left out, so the queues hold exactly what the read
// loop handed over.
func TestASessionsCopiesReachOneFramingWorkerInReadOrder(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "li-x3.sock")

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Skipf("unixpacket sockets unavailable here: %v", err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup

	// Not a multiple of the worker count, so a session-blind rotation over the workers
	// cannot happen to keep each session on one.
	const sessions, perSession = 13, 20

	s := &liShipper{
		sockAddr: addr,
		senders:  make(map[string]x2x3.Sender),
		punted:   make([]chan []byte, liFrameWorkers),
		free:     make(chan []byte, sessions*perSession),
	}
	for i := range s.punted {
		s.punted[i] = make(chan []byte, sessions*perSession)
	}
	s.egressDown.Store(true)

	go s.shipLoop()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("the shipper never dialled the egress: %v", err)
	}
	defer conn.Close() //nolint:errcheck // test cleanup

	// Interleaved across sessions, as the datapath tees them. The SEIDs are consecutive,
	// so they fall on every worker. Each copy carries its session and its place in that
	// session's stream.
	for i := range perSession {
		for seid := range uint64(sessions) {
			copyOf := make([]byte, liTagLen+2)
			binary.LittleEndian.PutUint64(copyOf, seid+1)
			copyOf[fseidTagLen] = farForwardUAndDuplicate
			copyOf[liTagLen] = byte(i)
			if _, err := conn.Write(copyOf); err != nil {
				t.Fatalf("writing copy %d of session %d: %v", i, seid+1, err)
			}
		}
	}

	queueOf := make(map[uint64]int, sessions)
	next := make(map[uint64]byte, sessions)
	deadline := time.After(10 * time.Second)

	for got := 0; got < sessions*perSession; {
		for q, punted := range s.punted {
			select {
			case b := <-punted:
				got++
				seid := binary.LittleEndian.Uint64(b)
				if first, seen := queueOf[seid]; seen && first != q {
					t.Fatalf("session %d's copies went to workers %d and %d", seid, first, q)
				}
				queueOf[seid] = q
				if b[liTagLen] != next[seid] {
					t.Fatalf("session %d: copy %d reached its worker where copy %d was due", seid, b[liTagLen], next[seid])
				}
				next[seid]++
			case <-deadline:
				t.Fatalf("only %d of %d copies reached the framing queues", got, sessions*perSession)
			default:
			}
		}
	}

	workers := make(map[int]bool)
	for _, q := range queueOf {
		workers[q] = true
	}
	if len(workers) < 2 {
		t.Errorf("%d sessions all went to one worker", sessions)
	}
}
