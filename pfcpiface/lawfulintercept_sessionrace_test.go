// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// liStoredSession builds a session allocated the way the store allocates one.
//
// **The allocation shape is the whole point.** NewPFCPSession makes each rule slice at
// cap(MaxItems) and nothing ever grows past it, so append never reallocates and a
// stored session's array is the same array every reader holds. A session built from
// slice literals — which is how every other test here builds one — has cap == len, so
// the first append reallocates and the sharing quietly stops being total, so -race
// would not see a race on the shared array: such a test does not reproduce the memory
// layout the store produces.
func liStoredSession(seid uint64, ue string) PFCPSession {
	addr := ip2int(net.ParseIP(ue))
	s := PFCPSession{
		localSEID:  seid,
		remoteSEID: seid + 1,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0, MaxItems),
			fars: make([]far, 0, MaxItems),
			qers: make([]qer, 0, MaxItems),
		},
	}
	s.CreatePDR(uplinkPDR(seid, 1, uint32(seid)+0x1000, ip2int(net.ParseIP("10.76.0.2")), addr))
	s.CreatePDR(downlinkPDR(seid, 2, addr))
	for id := uint32(1); id <= 4; id++ {
		s.CreateFAR(far{farID: id, fseID: seid, applyAction: ActionForward})
	}

	return s
}

// modifyFARRequest is one round of the modification the SMF sends, and it alternates
// so that every round writes.
//
// Both writes matter and they are different writes: UpdateFAR replaces a whole far
// inside the array, and RemoveFAR shifts the remainder down inside it — the second is
// what lets a reader observe a rule at an index that now belongs to a different one.
// A round that only removed would fail on the next round with the FAR already gone,
// and the handler answers a failed removal by rolling the round back, so the writer
// would leave the session unchanged after one round and the test would assert nothing.
func modifyFARRequest(seid uint64, round int) message.Message {
	seq := uint32(round)
	if round%2 == 0 {
		return message.NewSessionModificationRequest(0, 0, seid, seq, 0,
			ie.NewUpdateFAR(
				ie.NewFARID(uint32(1+round%3)),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
			),
			ie.NewRemoveFAR(ie.NewFARID(4)),
		)
	}

	// Put it back, so the next even round has something to remove.
	return message.NewSessionModificationRequest(0, 0, seid, seq, 0,
		ie.NewCreateFAR(
			ie.NewFARID(4),
			ie.NewApplyAction(ActionForward),
			ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
		),
	)
}

// modifyRepeatedly drives the writer and answers how many modifications the handler
// actually applied. The count is asserted, because a handler that returns early — as
// it does on a removal it cannot perform — writes nothing, and a writer that stops
// writing turns this into a test of nothing at all.
func modifyRepeatedly(pConn *PFCPConn, seid uint64, rounds int) int {
	applied := 0
	for i := range rounds {
		rsp, err := pConn.handleSessionModificationRequest(modifyFARRequest(seid, i))
		if err != nil {
			continue
		}
		if smres, ok := rsp.(*message.SessionModificationResponse); ok && smres.Cause != nil {
			if cause, err := smres.Cause.Cause(); err == nil && cause == ie.CauseRequestAccepted {
				applied++
			}
		}
	}

	return applied
}

// raceConn is the minimum PFCPConn the modification handler needs: a store to read the
// session from, and a datapath to answer the push.
func raceConn(t *testing.T, sessions SessionsStore) *PFCPConn {
	t.Helper()

	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() }) //nolint:errcheck // test cleanup

	return &PFCPConn{Conn: l, store: sessions, upf: &upf{datapath: &fakeDP{}}}
}

// TestAStoredSessionsRulesAreStableForAConcurrentReader drives the two paths that
// reach one session's rules with no ordering between them, against the real store,
// under -race.
//
// The interception plane re-derives duplication on its own worker, reading sess.fars
// for every session the store holds. The session's own signalling modifies those same
// FARs in place. Both are correct in isolation and the ordering arguments above them
// are sound — they operate on whole sessions, and this operates on the bytes of one
// rule. What a reader can observe is a far half-replaced, or a far at an index that
// now belongs to a different rule, and what it does with it is program BESS: the wrong
// forwarding action, or the wrong duplication decision, for a rule belonging to
// someone else's traffic.
//
// It must fail before the copy-on-write. A test that builds two sessions and asserts
// they do not alias would pass against the broken build, because the aliasing is
// between a stored session and the copy every getter hands out.
func TestAStoredSessionsRulesAreStableForAConcurrentReader(t *testing.T) {
	sessions := NewInMemoryStore()

	sess := liStoredSession(100, testUEIPv4)
	if cap(sess.fars) != MaxItems {
		t.Fatalf("the fixture allocates fars at cap %d, not the store's %d; "+
			"it no longer reproduces the sharing this test exists for", cap(sess.fars), MaxItems)
	}
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	e := newCCEnabler(store.New(), confirmedPush(func(_, _ PacketForwardingRules) uint8 {
		return ie.CauseRequestAccepted
	}), nil)
	t.Cleanup(e.stop)
	e.addSource(sessions)

	pConn := raceConn(t, sessions)

	var wg sync.WaitGroup

	const rounds = 60

	applied := 0

	wg.Add(2)
	go func() {
		defer wg.Done()
		applied = modifyRepeatedly(pConn, 100, rounds)
	}()
	go func() {
		defer wg.Done()
		for range rounds {
			e.retaskAndWait()
		}
	}()
	wg.Wait()

	if applied < rounds/2 {
		t.Fatalf("the handler applied %d of %d modifications; it stopped writing, "+
			"so nothing was racing the re-derivation", applied, rounds)
	}
}

// TestTheFramingPathReadsAStableSession is the same hazard reached from the other
// reader, and the one that runs per packet.
//
// resolveCovering resolves a duplicated copy's F-SEID to the task covering it, and gets
// there through sessionFor — a plain GetSession, on a framing worker. It then resolves
// detection criteria against that session's rules. So the second reader of the arrays
// the modification handler writes is on the content path, where a torn read decides
// which warrant a packet is delivered under.
func TestTheFramingPathReadsAStableSession(t *testing.T) {
	sessions := NewInMemoryStore()

	sess := liStoredSession(200, "10.250.0.10")
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	tasks := store.New()
	e := newCCEnabler(tasks, confirmedPush(func(_, _ PacketForwardingRules) uint8 {
		return ie.CauseRequestAccepted
	}), nil)
	t.Cleanup(e.stop)
	e.addSource(sessions)

	task := ccTask("W1", ueAddr("10.250.0.10"))
	if !tasks.Activate(task) {
		t.Fatal("Activate failed")
	}
	e.retaskAndWait()

	pConn := raceConn(t, sessions)

	var wg sync.WaitGroup

	const rounds = 60

	applied := 0

	wg.Add(2)
	go func() {
		defer wg.Done()
		applied = modifyRepeatedly(pConn, 200, rounds)
	}()
	go func() {
		defer wg.Done()
		for range rounds {
			// The epoch is bumped so the memo does not answer from cache: what has to
			// run concurrently with the writer is the resolution itself.
			e.epoch.Add(1)
			e.resolveCovering(200)
		}
	}()
	wg.Wait()

	if applied < rounds/2 {
		t.Fatalf("the handler applied %d of %d modifications; it stopped writing, "+
			"so nothing was racing the framing path", applied, rounds)
	}
}

// qerSession is liStoredSession with the one thing the other two fixtures lack: two QERs,
// and PDRs whose qerIDList names both of them.
//
// Without both, MarkSessionQer returns at its own guard — "need at least 1 QER in PDR or
// 2 QERs in session" — so the write this test exists for never happens. That is exactly
// why the existing race tests missed it: they carry FAR IEs only.
func qerSession(seid uint64, ue string) PFCPSession {
	addr := ip2int(net.ParseIP(ue))
	s := PFCPSession{
		localSEID:  seid,
		remoteSEID: seid + 1,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0, MaxItems),
			fars: make([]far, 0, MaxItems),
			qers: make([]qer, 0, MaxItems),
		},
	}

	up := uplinkPDR(seid, 1, uint32(seid)+0x1000, ip2int(net.ParseIP("10.76.0.2")), addr)
	down := downlinkPDR(seid, 2, addr)
	// Both QERs in both lists, allocated at capacity so the in-place shift
	// MarkSessionQer performs stays inside the array every reader holds — the same
	// reason liStoredSession allocates the rule slices at cap(MaxItems).
	for _, p := range []*pdr{&up, &down} {
		list := make([]uint32, 0, 4)
		p.qerIDList = append(list, 4, 5)
	}
	s.CreatePDR(up)
	s.CreatePDR(down)

	for id := uint32(1); id <= 4; id++ {
		s.CreateFAR(far{farID: id, fseID: seid, applyAction: ActionForward})
	}
	// A session QER and an application QER: MarkSessionQer picks the one with the larger
	// MBR and no GBR, then moves its id to the end of every PDR's list.
	s.CreateQER(qer{qerID: 4, fseID: seid, ulMbr: 100000, dlMbr: 100000})
	s.CreateQER(qer{qerID: 5, fseID: seid, ulMbr: 50000, dlMbr: 50000})

	return s
}

// modifyQERRequest is a modification carrying QER IEs, so the handler reaches
// MarkSessionQer with two QERs in the session and a qerIDList in every PDR.
func modifyQERRequest(seid uint64, round int) message.Message {
	mbr := uint64(100000 + round*1000)

	return message.NewSessionModificationRequest(0, 0, seid, uint32(round), 0,
		ie.NewUpdateQER(
			ie.NewQERID(4),
			ie.NewGateStatus(ie.GateStatusOpen, ie.GateStatusOpen),
			ie.NewMBR(mbr, mbr),
			ie.NewGBR(0, 0),
		),
		ie.NewUpdateQER(
			ie.NewQERID(5),
			ie.NewGateStatus(ie.GateStatusOpen, ie.GateStatusOpen),
			ie.NewMBR(mbr/2, mbr/2),
			ie.NewGBR(0, 0),
		),
	)
}

// TestAStoredSessionsQERListIsStableForAConcurrentReader is the same hazard as its two
// siblings, one level below where the copy was made.
//
// The copy-on-write is genuine and one level deep: copying the rule slices copies each
// pdr struct, and a pdr's qerIDList is itself a slice whose backing array the copy still
// shares. MarkSessionQer shifts that array in place on every session modification —
// removing the session QER's id and appending it at the end — while resolveCovering
// evaluates a TargetQERID criterion against it from a framing worker.
//
// The other race tests cannot see it: they carry FAR IEs only, so MarkSessionQer returns
// at its two-QER guard and the write never happens. This one carries QER IEs and a
// session with two QERs, which is what a real modification of a policed session looks
// like.
//
// Must fail before the deep copy, under -race.
func TestAStoredSessionsQERListIsStableForAConcurrentReader(t *testing.T) {
	sessions := NewInMemoryStore()

	sess := qerSession(300, "10.250.0.11")
	if len(sess.pdrs[0].qerIDList) != 2 || len(sess.qers) != 2 {
		t.Fatalf("the fixture has %d QER ids in the first PDR and %d QERs in the session, want 2 "+
			"and 2; MarkSessionQer returns at its guard otherwise and this test asserts nothing",
			len(sess.pdrs[0].qerIDList), len(sess.qers))
	}
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	tasks := store.New()
	e := newCCEnabler(tasks, confirmedPush(func(_, _ PacketForwardingRules) uint8 {
		return ie.CauseRequestAccepted
	}), nil)
	t.Cleanup(e.stop)
	e.addSource(sessions)

	// A criterion that reads the list: a QER's traffic, which selects every PDR the QER
	// polices. TargetPDR would reach it too — a whole-rule comparison reads qerIDList —
	// but this is the criterion whose whole evaluation is that read.
	task := ccTask("W1", types.TargetIdentifier{Type: types.TargetQERID, Value: "5"})
	if !tasks.Activate(task) {
		t.Fatal("Activate failed")
	}
	e.retaskAndWait()

	pConn := raceConn(t, sessions)

	var wg sync.WaitGroup

	const rounds = 60

	applied := 0

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range rounds {
			rsp, err := pConn.handleSessionModificationRequest(modifyQERRequest(300, i))
			if err != nil {
				continue
			}
			if smres, ok := rsp.(*message.SessionModificationResponse); ok && smres.Cause != nil {
				if cause, err := smres.Cause.Cause(); err == nil && cause == ie.CauseRequestAccepted {
					applied++
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range rounds {
			// The epoch is bumped so the memo does not answer from cache: what has to run
			// concurrently with the writer is the resolution itself.
			e.epoch.Add(1)
			e.resolveCovering(300)
		}
	}()
	wg.Wait()

	if applied < rounds/2 {
		t.Fatalf("the handler applied %d of %d modifications; it stopped writing, so nothing was "+
			"racing the framing path", applied, rounds)
	}
}

// liRecordingDP is a datapath that remembers the last body programmed for each FAR, so a
// test can assert on what the user plane was actually left holding rather than on which
// call happened.
type liRecordingDP struct {
	fakeDP

	mu   sync.Mutex
	last map[uint32]far
}

func newRecordingDP() *liRecordingDP {
	return &liRecordingDP{last: make(map[uint32]far)}
}

func (d *liRecordingDP) SendMsgToUPF(_ upfMsgType, _ PacketForwardingRules, updated PacketForwardingRules) uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, f := range updated.fars {
		d.last[f.farID] = f
	}

	return ie.CauseRequestAccepted
}

// SendMsgToUPFWithCompletion routes through SendMsgToUPF, which the embedded fakeDP's version
// would not reach.
func (d *liRecordingDP) SendMsgToUPFWithCompletion(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	return d.SendMsgToUPF(method, all, updated), true
}

func (d *liRecordingDP) lastFor(farID uint32) (far, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	f, ok := d.last[farID]

	return f, ok
}

// TestATaskingPassDoesNotRestateAStaleForwardingBody is the second half of the same
// invariant as the copy: a published session's rules must not be *restated* from a
// snapshot either.
//
// The plan and the push are not one step. transact reads the session's FARs under its
// own lock, drops the lock — it has to, since the push is a round trip to the datapath
// and holding the lock across it would serialise every session handler behind one gRPC
// call — and then pushes the bodies it planned. In that interval the session path can
// complete its own read-modify-write of the same FAR, program the datapath with the new
// body, and record the write. The pass then restated the body it planned from the
// replaced snapshot: the interception plane corrupting the subscriber's own forwarding,
// which transact's own comment says it must never do.
//
// Driven through the deterministic seam rather than by racing two goroutines and hoping
// for the bad order: the interleaving is a few instructions wide, and a property this
// consequential asserted by hope is one that passes against the defect. What the seam
// arranges is precisely the case the remedy addresses — the modification completes,
// PutSession included, between the pass's plan and the pass's push. The other order, a
// modification starting while the pass is pushing, is
// TestATaskingPushAndAModificationDoNotInterleave.
func TestATaskingPassDoesNotRestateAStaleForwardingBody(t *testing.T) {
	sessions := NewInMemoryStore()
	sess := liStoredSession(400, "10.250.0.12")
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	dp := newRecordingDP()

	tasks := store.New()
	e := newCCEnabler(tasks, confirmedPush(func(all, updated PacketForwardingRules) uint8 {
		return dp.SendMsgToUPF(upfMsgTypeMod, all, updated)
	}), nil)
	t.Cleanup(e.stop)
	e.addSource(sessions)

	// The enabler is wired into the connection, as production wires it: without it
	// applyTasking and sessionProgrammed are nil-receiver no-ops, so the modification
	// would never record its write and the pass would have nothing to notice.
	pConn := &PFCPConn{Conn: nil, store: sessions, upf: &upf{datapath: dp, ccEnabler: e}}

	// The SMF's modification runs here: once, between the pass's plan and its push. It
	// replaces FAR 1's forwarding body and programs the datapath itself.
	var once sync.Once
	beforeTransactPush = func() {
		once.Do(func() {
			rsp, err := pConn.handleSessionModificationRequest(
				message.NewSessionModificationRequest(0, 0, 400, 1, 0,
					ie.NewUpdateFAR(
						ie.NewFARID(1),
						ie.NewApplyAction(ActionForward),
						ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
					),
				))
			if err != nil {
				t.Errorf("the modification failed, so nothing raced the pass: %v", err)

				return
			}
			smres, ok := rsp.(*message.SessionModificationResponse)
			if !ok || smres.Cause == nil {
				t.Error("the modification produced no cause, so nothing raced the pass")

				return
			}
			if cause, err := smres.Cause.Cause(); err != nil || cause != ie.CauseRequestAccepted {
				t.Errorf("the modification was refused (cause %d), so nothing raced the pass", cause)
			}
		})
	}
	t.Cleanup(func() { beforeTransactPush = nil })

	// Tasking that makes the pass touch this session's FARs at all.
	if !tasks.Activate(ccTask("W1", ueAddr("10.250.0.12"))) {
		t.Fatal("Activate failed")
	}
	e.retaskAndWait()

	got, ok := dp.lastFor(1)
	if !ok {
		t.Fatal("FAR 1 was never programmed; this test asserts nothing")
	}
	if got.dstIntf != ie.DstInterfaceCore {
		t.Errorf("the datapath was last programmed with dstIntf %d for FAR 1, want %d "+
			"(the modification's): a tasking pass restated a forwarding body it planned from a "+
			"snapshot the SMF had already replaced, so the interception plane corrupted the "+
			"subscriber's own forwarding", got.dstIntf, ie.DstInterfaceCore)
	}
}

// TestASessionDeletedDuringAPassIsNotReAddedToTheDatapath is the third writer this
// invariant has to survive, and the one the per-FAR ordering cannot express.
//
// A pass takes its mark, plans a session's FAR bodies, drops the lock and pushes. A
// *modification* landing in that interval is caught by comparing each FAR's write stamp
// against the mark. A *deletion* is not: the session has no FARs left to compare stamps
// against, and `sessionForgotten` removed its entries from the record without moving the
// write counter — so the carry-over could not see the deletion, `e.programmed = fresh` put
// the pass's own planned entries back, and the push re-added the FAR to the datapath after
// the delete. Duplication reinstated on a session that has been torn down, recorded as
// programmed, with nothing left to turn it off.
//
// **Asserted on the datapath operations rather than on the `programmed` map**, because the
// map is this element's belief and the datapath is the fact: a fix that tidied the map while
// still pushing would pass a map-level test and leave a subscriber's traffic being copied.
func TestASessionDeletedDuringAPassIsNotReAddedToTheDatapath(t *testing.T) {
	sessions := NewInMemoryStore()
	sess := liStoredSession(500, "10.250.0.14")
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	dp := newRecordingDP()

	tasks := store.New()
	e := newCCEnabler(tasks, confirmedPush(func(all, updated PacketForwardingRules) uint8 {
		return dp.SendMsgToUPF(upfMsgTypeMod, all, updated)
	}), nil)
	t.Cleanup(e.stop)
	e.addSource(sessions)

	// The session is deleted between the pass's plan and its push, which is the interleaving
	// the dropped lock admits. Driven through the seam rather than by racing goroutines: the
	// window is a few instructions wide.
	var once sync.Once
	beforeTransactPush = func() {
		once.Do(func() {
			// Exactly what the deletion handler does: remove the session from the store and
			// tell the interception plane it has gone.
			if err := sessions.DeleteSession(500); err != nil {
				t.Errorf("DeleteSession: %v", err)
			}
			e.sessionForgotten(&sess)
		})
	}
	t.Cleanup(func() { beforeTransactPush = nil })

	if !tasks.Activate(ccTask("W1", ueAddr("10.250.0.14"))) {
		t.Fatal("Activate failed")
	}
	e.retaskAndWait()

	if _, programmed := dp.lastFor(1); programmed {
		t.Error("a FAR was programmed into the datapath for a session that had already been " +
			"deleted: the pass planned it, the session went, and the push re-added duplication " +
			"to a session that no longer exists — so nothing will ever turn it off")
	}

	// And the record does not resurrect it either, which is what the next pass differences
	// against.
	e.mu.Lock()
	held := 0
	for ref := range e.programmed {
		if ref.seid == 500 {
			held++
		}
	}
	e.mu.Unlock()

	if held != 0 {
		t.Errorf("the record holds %d entries for a deleted session: the pass's own map put "+
			"them back over the deletion", held)
	}
}

// liTableDP is a datapath that holds the FARs it was told about the way a table does: a
// write puts each FAR it carries, a delete takes out each FAR it names. It answers "what
// does the user plane hold at the end", which a record of the last write per FAR cannot
// when the last operation was a delete.
type liTableDP struct {
	fakeDP

	mu   sync.Mutex
	fars map[uint32]far
}

func newTableDP() *liTableDP {
	return &liTableDP{fars: make(map[uint32]far)}
}

func (d *liTableDP) SendMsgToUPF(method upfMsgType, all, updated PacketForwardingRules) uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()

	if method == upfMsgTypeDel {
		for _, f := range all.fars {
			delete(d.fars, f.farID)
		}

		return ie.CauseRequestAccepted
	}

	for _, f := range updated.fars {
		d.fars[f.farID] = f
	}

	return ie.CauseRequestAccepted
}

func (d *liTableDP) SendMsgToUPFWithCompletion(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	return d.SendMsgToUPF(method, all, updated), true
}

func (d *liTableDP) holds(farID uint32) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, ok := d.fars[farID]

	return ok
}

// heldPush is a tasking pass's push whose first call stops on entry, after signalling
// entered, until release is called; it then writes through to the datapath. Later calls
// do not stop. The pass is then inside its push with the session's stripe held, which is
// the window a session handler must not complete its own write in.
//
// release is safe to call more than once, and is registered as a cleanup so a test that
// fails before releasing does not leave the worker blocked and stop waiting for it.
func heldPush(t *testing.T, write func(all, updated PacketForwardingRules) uint8) (
	push func(all, updated PacketForwardingRules) (uint8, bool), entered <-chan struct{}, release func(),
) {
	t.Helper()

	in := make(chan struct{})
	out := make(chan struct{})

	var first, released sync.Once

	release = func() { released.Do(func() { close(out) }) }

	push = func(all, updated PacketForwardingRules) (uint8, bool) {
		first.Do(func() {
			close(in)
			<-out
		})

		cause := write(all, updated)

		return cause, cause == ie.CauseRequestAccepted
	}

	return push, in, release
}

// awaitPush waits for a pass to enter its push.
func awaitPush(t *testing.T, entered <-chan struct{}) {
	t.Helper()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no tasking pass pushed this session's FARs, so nothing could interleave with it")
	}
}

// handlerGrace is how long a test lets a session handler run while a pass is held in its
// push. A handler that is not serialised against the push completes well inside it; one
// that is waits on the session's stripe for as long as the push is held.
const handlerGrace = 250 * time.Millisecond

// runAgainstHeldPush runs handler in a goroutine while a pass is held in its push, gives it
// handlerGrace, asserts that it has not finished, then releases the push and waits for the
// handler.
func runAgainstHeldPush(t *testing.T, release func(), handler func() error) {
	t.Helper()

	done := make(chan error, 1)

	go func() { done <- handler() }()

	var (
		err      error
		finished bool
	)

	select {
	case err = <-done:
		finished = true

		t.Error("the session handler finished while a pass was held in its push of the same " +
			"session: the handler is not serialised against the push, so its write and the " +
			"pass's can land in either order")
	case <-time.After(handlerGrace):
	}

	release()

	if !finished {
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the session handler did not complete once the pass's push was released")
		}
	}

	if err != nil {
		t.Fatalf("the session handler failed, so nothing interleaved with the pass: %v", err)
	}
}

// TestATaskingPushAndAModificationDoNotInterleave is the order
// TestATaskingPassDoesNotRestateAStaleForwardingBody does not reach: the modification starts
// after the pass has re-read the record and while its push is in flight.
//
// The pass's re-read passes, because the modification has not stamped anything yet. If the
// modification could then write its body to the datapath and stamp the record while the
// pass's push was still outstanding, the pass's body — planned from the snapshot the SMF has
// just replaced — would land last, and the datapath would forward the subscriber's traffic
// by the old body until the next modification. No later pass repairs it: a pass compares
// only whether a FAR is duplicating, and both bodies say it is.
//
// The push is held on entry, which is inside the stripe, so the interleaving is arranged
// rather than hoped for.
func TestATaskingPushAndAModificationDoNotInterleave(t *testing.T) {
	const seid = 600

	sessions := NewInMemoryStore()
	if err := sessions.PutSession(liStoredSession(seid, "10.250.0.16")); err != nil {
		t.Fatal(err)
	}

	dp := newRecordingDP()

	push, entered, release := heldPush(t, func(all, updated PacketForwardingRules) uint8 {
		return dp.SendMsgToUPF(upfMsgTypeMod, all, updated)
	})

	tasks := store.New()
	e := newCCEnabler(tasks, push, nil)
	t.Cleanup(e.stop)
	t.Cleanup(release)
	e.addSource(sessions)

	pConn := &PFCPConn{Conn: nil, store: sessions, upf: &upf{datapath: dp, ccEnabler: e}}

	if !tasks.Activate(ccTask("W1", ueAddr("10.250.0.16"))) {
		t.Fatal("Activate failed")
	}
	e.retask()
	awaitPush(t, entered)

	// The SMF moves FAR 1's forwarding while the pass's push of the old body is in flight.
	runAgainstHeldPush(t, release, func() error {
		rsp, err := pConn.handleSessionModificationRequest(
			message.NewSessionModificationRequest(0, 0, seid, 1, 0,
				ie.NewUpdateFAR(
					ie.NewFARID(1),
					ie.NewApplyAction(ActionForward),
					ie.NewUpdateForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
				),
			))
		if err != nil {
			return err
		}

		smres, ok := rsp.(*message.SessionModificationResponse)
		if !ok || smres.Cause == nil {
			return fmt.Errorf("the modification produced no cause")
		}

		if cause, err := smres.Cause.Cause(); err != nil || cause != ie.CauseRequestAccepted {
			return fmt.Errorf("the modification was refused (cause %d)", cause)
		}

		return nil
	})

	// The pass the modification asked for, so the assertions read a settled element.
	e.await(e.request())

	got, ok := dp.lastFor(1)
	if !ok {
		t.Fatal("FAR 1 was never programmed; this test asserts nothing")
	}

	if got.dstIntf != ie.DstInterfaceCore {
		t.Errorf("the datapath was last programmed with dstIntf %d for FAR 1, want %d (the "+
			"modification's): the pass's push of the body it planned before the modification "+
			"landed after the modification's own, so the subscriber's traffic follows a "+
			"forwarding body the SMF has replaced", got.dstIntf, ie.DstInterfaceCore)
	}

	if !got.liDuplicate {
		t.Error("the datapath's FAR 1 is not duplicating although the tasking selects it")
	}

	stored, ok := sessions.GetSession(seid)
	if !ok {
		t.Fatal("the session is gone from the store")
	}

	for _, f := range stored.fars {
		if f.farID == 1 && (f.dstIntf != ie.DstInterfaceCore || !f.liDuplicate) {
			t.Errorf("the stored FAR 1 is {dstIntf %d, liDuplicate %v}, want {%d, true}: the "+
				"session is not as the modification left it", f.dstIntf, f.liDuplicate, ie.DstInterfaceCore)
		}
	}

	e.mu.Lock()
	rec := e.programmed[farRef{seid: seid, farID: 1}]
	e.mu.Unlock()

	if !rec.duplicating {
		t.Error("the record says FAR 1 is not duplicating although the datapath was last told it is")
	}
}

// TestATaskingPushAndADeletionDoNotInterleave is the same window with a deletion in it. A
// pass held in its push of FAR 1 has already re-read the record and found the session
// alive; if the deletion could remove the session's rules from the datapath and forget it
// in that window, the pass's push would land afterwards and re-create FAR 1 — a FAR the
// datapath holds for a session nothing in this element knows of, and nothing will ever
// remove.
func TestATaskingPushAndADeletionDoNotInterleave(t *testing.T) {
	const seid = 700

	sess := liStoredSession(seid, "10.250.0.18")
	// RemoveSession counts the session out of the metrics.
	sess.metrics = metrics.NewSession(testRemoteNodeID)

	sessions := NewInMemoryStore()
	if err := sessions.PutSession(sess); err != nil {
		t.Fatal(err)
	}

	dp := newTableDP()

	push, entered, release := heldPush(t, func(all, updated PacketForwardingRules) uint8 {
		return dp.SendMsgToUPF(upfMsgTypeMod, all, updated)
	})

	tasks := store.New()
	e := newCCEnabler(tasks, push, nil)
	t.Cleanup(e.stop)
	t.Cleanup(release)
	e.addSource(sessions)

	pConn := &PFCPConn{
		store:          sessions,
		upf:            &upf{datapath: dp, ccEnabler: e},
		InstrumentPFCP: noopMetrics{},
	}

	if !tasks.Activate(ccTask("W1", ueAddr("10.250.0.18"))) {
		t.Fatal("Activate failed")
	}
	e.retask()
	awaitPush(t, entered)

	runAgainstHeldPush(t, release, func() error {
		rsp, err := pConn.handleSessionDeletionRequest(
			message.NewSessionDeletionRequest(0, 0, seid, 1, 0))
		if err != nil {
			return err
		}

		sdres, ok := rsp.(*message.SessionDeletionResponse)
		if !ok || sdres.Cause == nil {
			return fmt.Errorf("the deletion produced no cause")
		}

		if cause, err := sdres.Cause.Cause(); err != nil || cause != ie.CauseRequestAccepted {
			return fmt.Errorf("the deletion was refused (cause %d)", cause)
		}

		return nil
	})

	e.await(e.request())

	for _, f := range sess.fars {
		if dp.holds(f.farID) {
			t.Errorf("the datapath holds FAR %d of a session that has been deleted: the pass's "+
				"push landed after the deletion's removal and re-created it, and nothing in this "+
				"element names it any more", f.farID)
		}
	}

	e.mu.Lock()
	held := 0
	for ref := range e.programmed {
		if ref.seid == seid {
			held++
		}
	}
	e.mu.Unlock()

	if held != 0 {
		t.Errorf("the record holds %d entries for a deleted session", held)
	}
}

// TestLockSessionsTakesASharedStripeOnce holds two sessions that share a stripe. lockSessions
// must take that stripe once, or it waits on itself, and release it once: a second release is
// a fatal error of the runtime, and none leaves the stripe held, which the lockSession
// afterwards would wait on.
func TestLockSessionsTakesASharedStripeOnce(t *testing.T) {
	e := &ccEnabler{}

	const first = uint64(1)

	second := first + sessionStripes

	done := make(chan struct{})

	go func() {
		defer close(done)

		unlock := e.lockSessions([]uint64{first, second})
		unlock()

		e.lockSession(first)()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("lockSessions with two sessions on one stripe deadlocked, or did not release " +
			"the stripe")
	}
}
