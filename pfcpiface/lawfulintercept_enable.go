// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/wmnsk/go-pfcp/ie"
)

// A triggered CC-POI cannot rely on the SMF having marked the traffic it is told to
// intercept. Only one of the detection criteria of TS 33.128 table 6.2.3-7 — the
// PFCP Session ID — is something our own CC Triggering Function derives from a
// session it has already marked; a criterion naming an address, a tunnel or a
// network instance may well identify traffic nothing has asked to be duplicated.
//
// Acknowledging such a task and producing nothing is the failure this file exists
// to prevent: the triggering function has been told the interception is running and
// has no way to discover that it is not. So the CC-POI enables duplication itself,
// on the FARs the criteria resolve to.
//
// Duplication is an apply-action on a FAR, and several PDRs may forward through
// one, so enabling it can copy more traffic than the criteria identify. That is
// accepted here and corrected downstream by filtering: the alternative — cloning a
// FAR and repointing a PDR at it — mutates the subscriber's own forwarding, where
// any mistake is visible to the target.
//
// The tasking is the single source of truth. Nothing here remembers which FARs it
// enabled: every point that could change the answer re-derives it from the live
// tasks and the session's current rules. A remembered set would be one more thing
// to drift out of step with reality, and its drifting would be silent.

// ccEnabler enables duplication for LI_T3 tasking on the FARs a task's detection
// criteria select, and keeps it enabled across the SMF's own session
// modifications.
type ccEnabler struct {
	tasks *store.Store

	// parsed is the live tasking with every task's detection criteria already parsed,
	// as an immutable snapshot behind an atomic pointer.
	//
	// The shipping path reads it per duplicated copy, on four workers. Parsing there
	// meant a strconv or a hex decode per criterion per copy — and for a PDR criterion
	// an IE parse and a throwaway address pool per copy — on the one path in this
	// element whose cost is per packet, in front of a socket queue that holds ten
	// datagrams by default and whose overflow is intercept product. A snapshot swapped
	// on tasking changes costs the hot path one atomic load.
	parsed atomic.Pointer[[]parsedTask]

	// epoch changes when the tasking changes, and stamps the memo below. A memo entry
	// from an older epoch is recomputed rather than trusted, so the memo cannot answer
	// from tasking that has been withdrawn — and a writer that forgot to bump this
	// would invalidate nothing rather than invalidate one thing wrongly, which is the
	// failure direction an index maintained incrementally does not have.
	epoch atomic.Uint64

	// coveredMu guards covered, which memoises the per-session attribution answer.
	// Separate from mu because it is taken per copy and mu is held across walks of
	// every session this element holds.
	coveredMu sync.Mutex
	covered   map[uint64]coveredEntry

	mu sync.Mutex
	// sources are the per-association session stores. A task's criteria are
	// resolved against every session this element holds, since most criteria do not
	// name one.
	sources []SessionsStore
	// programmed records which FARs this element has told the datapath to duplicate.
	// It is not the authority on what *should* be duplicated — that is recomputed
	// from the live tasking every time — only a record of what the datapath was last
	// told, so that a re-derivation producing the same answer does not reprogram it.
	//
	// It is nonetheless load-bearing in one direction: a re-derivation skips a FAR
	// whose recorded value already equals what the tasking implies, so a *missing*
	// entry is an instruction to do nothing — and doing nothing is wrong exactly
	// when the datapath is duplicating and the tasking says it should not be.
	programmed map[farRef]programmedFAR
	// writes counts writes to programmed, and is what lets a re-derivation tell an
	// entry the session path wrote while it ran from one it read itself.
	//
	// Deliberately not requested/served/completed. Those count *requests for a
	// re-derivation*; this counts *writes to the record*. A pass serving generation
	// G would see its own G on entries written during it and discard them anyway,
	// so conflating the two silently defeats the carry-over below.
	writes uint64
	// forgotten records, per session, the write stamp at which this element learned the
	// session was gone. It is what lets a pass planned from an older view be told that the
	// session it planned for no longer exists: the pass's own map still holds the entries
	// it built, and without this the carry-over could not see the deletion — so
	// `e.programmed = fresh` put them back and the pass's push could re-add the FAR to the
	// datapath after the delete.
	//
	// Entries are dropped by the pass that acts on them, so this is bounded by the
	// deletions a single pass can miss rather than by uptime.
	forgotten map[uint64]uint64
	// everDuplicated is every FAR of a live session this element has ever told the datapath to
	// duplicate.
	//
	// **It exists because `programmed` is the only memory of what the datapath holds, and a
	// record that is wrong in the "not duplicating" direction is unrecoverable.** The
	// re-derivation skips any FAR whose recorded value already equals what the tasking implies,
	// so once the record says "off" while the datapath is duplicating, no later pass mentions
	// that FAR again: the element computes "nothing should duplicate", sees its record agreeing,
	// and does nothing while a subscriber's traffic goes on being copied. Nothing on either side
	// can see it — the element's own account says duplication is off, and the copies are dropped
	// as unattributable rather than delivered, so no agency sees them either.
	//
	// That state has been observed on a deployment and what produced it is not established, so
	// this set is deliberately not a fix for any particular path. It is a fix for the *permanence*, which is the part that makes the
	// harm unbounded — with it, the element tells the datapath "off" for every FAR it has ever
	// turned on, whatever it believes, so a divergence lasts until the next re-derivation
	// instead of until the pod restarts.
	//
	// **Monotone within a session's life, on purpose.** The bug class is an entry being wrongly
	// overwritten to false, and a set that only grows has no such path. Nothing removes an entry
	// while the FAR it names still exists.
	//
	// **It is reclaimed with that FAR, though**, in sessionForgotten and farsRemoved, and the
	// reason is that it cannot be read afterwards: transact consults this set only inside its
	// walk of live sessions' FARs, so an entry whose session or FAR is gone is unreachable and
	// retaining it buys nothing. A later session cannot inherit a stale entry by being allocated
	// the same SEID, because NewPFCPSession allocates from a 64-bit random source.
	//
	// Retaining them would not be free, either. Every record of what was programmed must stay
	// bounded, and this is one: a task keyed by network instance or tunnel direction selects
	// *every* session, so under ordinary subscriber churn an unpruned set would grow with every
	// session that has ever existed while the task was held. The element would keep intercepting
	// correctly the whole time, which is what would make it invisible until the process dies and
	// takes every warrant with it.
	everDuplicated map[farRef]bool
	// faultsMemo answers taskFaults without re-walking every session, stamped with the two
	// things the answer depends on: the tasking epoch, and the record's write counter — which
	// moves on every establishment, modification and teardown, because all three record what
	// they programmed. Guarded by faultsMu rather than mu, which is held across walks.
	faultsMu   sync.Mutex
	faultsMemo map[types.XID]faultEntry
	// forgottenFARs is the same record one level down: the write stamp at which a session
	// *modification* removed one FAR while the session itself carried on.
	//
	// A separate map because the two cannot be expressed as one. `forgotten` answers "is
	// anything about this SEID stale", which a pass uses to discard its whole conclusion for
	// that session — right when the session is gone, and wrong here: this pass may already
	// have pushed duplication for the session's *other* FARs, and discarding the record of
	// that push would leave the datapath duplicating with nothing able to turn it off. So a
	// removed FAR invalidates that FAR and nothing else.
	//
	// It is needed at all because a deletion is invisible to the carry-over, exactly as the
	// session-level case documents: without it the pass's own `fresh` map still holds the
	// entry it planned from the pre-modification FAR list, `e.programmed = fresh` puts it
	// back, and the record then claims duplication for a FAR the SMF has removed. The next
	// pass would compare against that claim and conclude there was nothing to do — and if
	// the session later re-creates a FAR with the same identifier, which a path switch
	// does routinely, the claim says duplication is already in place and the freshly created
	// FAR is never programmed. Interception silently stops for that traffic while this
	// element's own account says it is running.
	//
	// Bounded like the map above: entries are dropped by the pass that acts on them.
	forgottenFARs map[farRef]uint64
	// push writes changed rules to the datapath and answers with the datapath's
	// cause. Separate field so a test can observe what would be programmed without a
	// datapath.
	//
	// **The answer is the point.** This record exists so the element knows what to
	// change, and a re-derivation programs only the difference between what the
	// tasking implies and what the record says is in place. Recording a refused
	// program as done therefore does not lose one attempt — it removes the rule from
	// every subsequent difference, so nothing ever retries it. Duplication for that
	// traffic never happens, this element's own account says it is happening, and the
	// only event that could correct the record is a change in tasking.
	//
	// The second answer is whether the datapath acknowledged the write at all. An
	// accepted cause is not that: a batch whose RPC did not complete is answered
	// accepted on purpose, so that an uncertain result does not have the session torn
	// down. This record must take the other side of that -- an unconfirmed write is
	// not a programmed one -- or the rule drops out of every later difference exactly
	// as a refused one would. See lawfulintercept_confirmation.go.
	push func(all, updated PacketForwardingRules) (cause uint8, confirmed bool)
	// report surfaces an LI-plane fault to the ADMF. nil when no ADMF is configured.
	report func(issueType, description string)

	// The three counters that make a re-derivation atomic with respect to another,
	// and let a caller find out when the one it asked for has happened. requested is
	// bumped by every request; served is the request the running transaction is
	// answering for; completed is the last one that finished. A request is coalesced
	// rather than queued — a transaction is a full re-derivation from current
	// tasking, so N pending requests and one imply the same work, and what must hold
	// is that the state programmed is the state the last of them implies.
	requested uint64
	served    uint64
	completed uint64
	// passes counts the transactions actually performed, so "N requests are the same
	// work as one" is assertable rather than asserted.
	passes uint64
	// closing stops the worker taking new work and releases anyone waiting on a
	// generation that will now never arrive.
	closing bool
	// pending wakes the worker when a request arrives; settled wakes callers when a
	// transaction finishes. Both guarded by mu.
	pending *sync.Cond
	settled *sync.Cond
	worker  sync.WaitGroup

	// sessionLocks serialise a pass's push for one session against the session handlers
	// that write the same session's rules to the datapath. A pass holds its session's
	// stripe from its final re-read of the record through the push and the recording of
	// the push's answer; a session handler holds it from before its first datapath write
	// until the handler returns, so whatever it records of those writes (sessionProgrammed,
	// farsRemoved, farsAttempted, sessionForgotten, or the abandonment reports) is recorded
	// under it. So either the handler's stamp is in the record before the pass re-reads it
	// and the pass leaves those FARs alone, or the pass's push reaches the datapath before
	// the handler's does. See lockSession.
	//
	// Lock order: PFCPConn.sessionsMu, then a stripe, then mu. transact never takes
	// sessionsMu, and no session handler waits on the worker, so a pass holding a stripe
	// cannot be waiting on a handler that holds it.
	sessionLocks [sessionStripes]sync.Mutex
}

// sessionStripes is how many locks the sessions share. Two sessions on one stripe wait
// on each other only while a pass is pushing for one of them or a handler is writing one
// to the datapath; a power of two keeps the modulo a mask.
const sessionStripes = 64

// lockSession takes the stripe for one session and returns its release. On a nil enabler,
// where no interception is configured and no pass ever pushes, there is nothing to
// serialise against and it returns a no-op.
func (e *ccEnabler) lockSession(seid uint64) (unlock func()) {
	if e == nil {
		return func() {}
	}

	m := &e.sessionLocks[seid%sessionStripes]
	m.Lock()

	return m.Unlock
}

// lockSessions takes the stripes of several sessions at once, each once, in stripe order,
// and returns their release. The fixed order is what lets two callers hold several stripes
// without deadlocking each other; every other caller holds one stripe at a time.
func (e *ccEnabler) lockSessions(seids []uint64) (unlock func()) {
	if e == nil {
		return func() {}
	}

	var held [sessionStripes]bool
	for _, seid := range seids {
		held[seid%sessionStripes] = true
	}

	for i := range held {
		if held[i] {
			e.sessionLocks[i].Lock()
		}
	}

	return func() {
		for i := range held {
			if held[i] {
				e.sessionLocks[i].Unlock()
			}
		}
	}
}

// parsedTask is one content task with its criteria in the form the agent compares
// against PFCP state. Parsed when the tasking changes rather than when a copy
// arrives; a task whose criteria do not parse contributes none, which canApply
// already refused at tasking time.
type parsedTask struct {
	task     types.InterceptTask
	criteria []criterion
}

// coveredEntry is the attribution answer for one session, stamped with the tasking
// epoch it was computed under.
type coveredEntry struct {
	epoch uint64
	tasks []coveredTask
}

// programmedFAR is what this element last told the datapath about one FAR, and
// when it said so.
type programmedFAR struct {
	duplicating bool
	// written is the value of writes at the moment the entry was recorded.
	written uint64
}

func newCCEnabler(
	tasks *store.Store,
	push func(all, updated PacketForwardingRules) (uint8, bool),
	report func(issueType, description string),
) *ccEnabler {
	e := &ccEnabler{
		tasks:          tasks,
		everDuplicated: make(map[farRef]bool),
		faultsMemo:     make(map[types.XID]faultEntry),
		push:           push,
		report:         report,
		programmed:     make(map[farRef]programmedFAR),
		forgotten:      make(map[uint64]uint64),
		forgottenFARs:  make(map[farRef]uint64),
		covered:        make(map[uint64]coveredEntry),
	}
	e.reparse()
	e.pending = sync.NewCond(&e.mu)
	e.settled = sync.NewCond(&e.mu)
	e.worker.Add(1)
	go e.run()

	return e
}

// run performs re-derivations, one at a time and never concurrently with another.
//
// A single worker is what makes the transaction atomic with respect to itself.
// Provisioning requests arrive concurrently, and if only the individual reads and
// writes were protected the pass that began earlier could program and publish last
// — clearing duplication for a task that is still active, with nothing re-deriving
// until some unrelated event happened to trigger another pass. That loss is silent
// by nature: a dropped copy produces no record for any downstream function to miss,
// so it could persist for the life of the warrant.
//
// The alternative is a mutex around the transaction, which is simpler and wrong
// here: the transaction walks every session of every source, so an X1 request's
// latency would become a function of session count on the element holding the most
// sessions.
func (e *ccEnabler) run() {
	defer e.worker.Done()

	for {
		e.mu.Lock()
		for e.requested == e.served && !e.closing {
			e.pending.Wait()
		}
		if e.closing {
			e.mu.Unlock()

			return
		}
		// Taken before the transaction reads the tasking, so completed can only
		// understate which requests this pass answered for. A caller waiting on a
		// later generation then waits for the next pass, which is the safe direction:
		// the unsafe one is reporting that tasking has been programmed when it has not.
		gen := e.requested
		e.served = gen
		e.mu.Unlock()

		e.transact()

		e.mu.Lock()
		e.completed = gen
		e.passes++
		e.mu.Unlock()
		e.settled.Broadcast()
	}
}

// transactions is how many re-derivations have been performed.
func (e *ccEnabler) transactions() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.passes
}

// stop ends the worker. A transaction already in flight finishes — leaving the
// datapath half-programmed would be worse than waiting for one bounded pass — and
// nothing new starts. Callers waiting on a generation are released rather than left
// blocked on one that will never arrive. Safe to call more than once.
func (e *ccEnabler) stop() {
	if e == nil {
		return
	}

	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	e.pending.Broadcast()
	e.settled.Broadcast()

	e.worker.Wait()
}

// request registers a re-derivation and returns the generation it was given.
func (e *ccEnabler) request() uint64 {
	e.mu.Lock()
	e.requested++
	gen := e.requested
	e.mu.Unlock()
	e.pending.Signal()

	return gen
}

// await blocks until a transaction that answers for gen has finished.
func (e *ccEnabler) await(gen uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for e.completed < gen && !e.closing {
		e.settled.Wait()
	}
}

// addSource registers an association's session store, so tasking already held
// applies to its sessions and tasking that arrives later can find them.
func (e *ccEnabler) addSource(s SessionsStore) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sources = append(e.sources, s)
}

// removeSource forgets an association's session store when it goes away. Its
// sessions are gone with it, so no duplication needs undoing — but keeping the
// store would have this resolve criteria against sessions the datapath no longer
// has.
func (e *ccEnabler) removeSource(s SessionsStore) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, have := range e.sources {
		if have == s {
			e.sources = append(e.sources[:i], e.sources[i+1:]...)

			break
		}
	}
}

// canApply reports whether this CC-POI can carry out a task, and is what refuses
// one it cannot. It answers on two things: the task must require the product this
// point of interception makes, and every detection criterion must be something the
// agent can resolve against PFCP state.
//
// It deliberately does *not* require that the criteria select something now. A
// criterion may name a subscriber who has not attached yet, and refusing that
// would make tasking depend on the order in which the triggering function and the
// UE happen to act.
func (e *ccEnabler) canApply(task types.InterceptTask) error {
	if e == nil {
		// No datapath to resolve against, which happens only in a test that builds a
		// shipper directly. Answering "yes" keeps that path as it was.
		return nil
	}
	if !producesCC(task) {
		// Accepting it would cost in both directions: the triggering function is told an
		// interception is running that will deliver nothing, and the datapath duplicates
		// a subject's traffic so that every copy can be discarded for want of an X3
		// destination. Duplicating a subject's traffic is the act interception authority
		// licenses, and doing it for a task that did not ask for content is doing it
		// without that authority even though nothing leaves the element.
		return fmt.Errorf("li: task does not require content of communication, which is the only product this point of interception produces")
	}
	if len(task.Targets) == 0 {
		return fmt.Errorf("li: task carries no detection criteria")
	}
	for _, id := range task.Targets {
		if _, err := parseCriterion(id); err != nil {
			return err
		}
	}
	// Every criterion must match (TS 103 221-1 table 6.2.1.2-1), so two that no packet
	// satisfies together — two sessions, two directions, two ports — select nothing at
	// every moment. The same table has the element reject the combination rather than
	// acknowledge an interception that can never run.
	if a, b, ok := task.ContradictoryTargets(); ok {
		return fmt.Errorf("li: task names %s and %s, which no packet satisfies together, and "+
			"every criterion of a task must match", a.Type, b.Type)
	}

	return nil
}

// producesCC reports whether a task asks for the product this point of interception
// makes. Every place that reads tasking to decide what the datapath does asks this
// first, so a task admitted before canApply checked it cannot still cause
// duplication, and cannot take attribution of a copy away from a task that can
// actually be delivered for.
func producesCC(t types.InterceptTask) bool {
	return t.WantsProduct(types.ProductCC)
}

// criteriaOf returns the parsed criteria of every active task that requires content,
// one set per task, because a task's criteria are conjunctive and selection is decided
// per task. A task with a criterion that does not parse is left out whole: dropping the
// one criterion would select more than the task asks for. canApply refuses such a task
// at tasking time, so a leftover can only come from one installed before that check.
func (e *ccEnabler) criteriaOf(tasks []types.InterceptTask) [][]criterion {
	var out [][]criterion
	for _, t := range tasks {
		if !producesCC(t) {
			continue
		}
		if set, ok := parseAll(t); ok {
			out = append(out, set)
		}
	}

	return out
}

// parseAll parses every criterion of a task, reporting false if any does not parse or the
// task names none.
func parseAll(t types.InterceptTask) ([]criterion, bool) {
	if len(t.Targets) == 0 {
		return nil, false
	}
	set := make([]criterion, 0, len(t.Targets))
	for _, id := range t.Targets {
		c, err := parseCriterion(id)
		if err != nil {
			return nil, false
		}
		set = append(set, c)
	}

	return set, true
}

// wantedFARs returns the FARs of one session whose PDRs some task's criteria select.
func wantedFARs(sets [][]criterion, sess PFCPSession) map[uint32]bool {
	one := []PFCPSession{sess}
	want := make(map[uint32]bool, len(sess.fars))
	for _, set := range sets {
		for _, ref := range selection(set, one) {
			want[ref.farID] = true
		}
	}

	return want
}

// setTasks gives the enabler the task store and parses what it holds.
//
// Assigning the field alone would leave the parsed snapshot empty until the first
// tasking change, and until then every copy would fall through to the store lookup
// that only the session-identity criterion can answer — an interception narrower
// than the one accepted, for as long as nothing happened to change the tasking.
func (e *ccEnabler) setTasks(tasks *store.Store) {
	if e == nil {
		return
	}

	e.tasks = tasks
	e.reparse()
}

// reparse rebuilds the parsed snapshot from the live tasking and moves the epoch on.
//
// The epoch moves *after* the snapshot is published, so a memo entry stamped with the
// new epoch cannot have been computed from the old snapshot. The other order would
// let a copy arriving in between be memoised under the new stamp from the old
// tasking, and nothing would recompute it.
func (e *ccEnabler) reparse() {
	if e == nil || e.tasks == nil {
		return
	}

	tasks := e.tasks.Snapshot()
	fresh := make([]parsedTask, 0, len(tasks))

	for _, t := range tasks {
		if !producesCC(t) {
			// A task that does not require content must not take attribution of a copy
			// away from one that does — the caller takes the first covering task, and a
			// task with no X3 destination would swallow the whole stream of the warrant
			// that has one.
			continue
		}

		criteria, ok := parseAll(t)
		if !ok {
			continue
		}
		fresh = append(fresh, parsedTask{task: t, criteria: criteria})
	}

	e.parsed.Store(&fresh)
	e.epoch.Add(1)
}

// forgetCovered drops the memoised attribution answer for one session.
//
// Called where a session's own rules change or the session goes away, which is
// precise: those events affect that session's answer and no other's. A change in
// *tasking* affects every session's, and moves the epoch instead.
func (e *ccEnabler) forgetCovered(seid uint64) {
	if e == nil {
		return
	}

	e.coveredMu.Lock()
	defer e.coveredMu.Unlock()
	delete(e.covered, seid)
}

// applyTasking sets duplication on every FAR of the session whose traffic live
// tasking selects, and clears the duplication this element enabled on the rest.
//
// Both halves matter. Setting is what makes an accepted task intercept, including
// after an SMF modification has replaced the FAR with its own view of it. Clearing
// is what makes a withdrawn task stop — and clearing only liDuplicate is what
// keeps it from stopping duplication the SMF itself asked for.
//
// updated, when non-nil, is the subset of rules the caller is about to push to the
// datapath; FARs in it are changed alongside the session's own copies, because the
// datapath acts on what it is pushed and PacketForwardingRules holds values rather
// than pointers.
//
// It deliberately does not record what it derived. The caller does that, through
// sessionProgrammed, once the session is in the store — see there for why the
// ordering is the whole of this file's coherence.
func (e *ccEnabler) applyTasking(s *PFCPSession, updated *PacketForwardingRules) {
	if e == nil || s == nil {
		return
	}

	// One session, so the resolver is given just this one: a criterion selecting
	// another session's traffic is that session's business.
	want := wantedFARs(e.criteriaOf(e.tasks.Snapshot()), *s)

	for i := range s.fars {
		s.fars[i].liDuplicate = want[s.fars[i].farID]
	}
	if updated != nil {
		for i := range updated.fars {
			updated.fars[i].liDuplicate = want[updated.fars[i].farID]
		}
	}
}

// sessionProgrammed records what the datapath was told about a session's FARs, and
// asks for a re-derivation where what it was told is something the element must be
// able to withdraw.
//
// **It must be called after the session is in the store, and that ordering is the
// whole point.** Recording inside applyTasking — before the push and before the
// store — leaves an interval in which the record says a session's traffic is being
// duplicated and GetAllSessions does not yet return it. A re-derivation beginning
// in that interval reads one structure and writes the other, and no stamp can save
// it: the write it would have to be newer than has already happened. Recording once
// the session is visible is what makes "written after this pass began looking" imply
// "this pass cannot have seen the session", which is what transact's carry-over
// relies on.
//
// The same ordering is why transact never acts on a value the session path wrote
// mid-flight. If it did, the pass would compare its own stale conclusion against the
// newer record, find a difference, and push the FAR *off* — ending an interception
// the tasking still required, with nothing logged and the triggering function still
// believing it was running.
//
// The re-derivation is asked for here rather than by the pass that lost the race,
// because here is the only place ordered after PutSession. It is gated so that an
// element holding no tasking never asks for one: every FAR reads false, nothing
// changed, and the walk of every session that a request implies does not happen.
//
// **pushed is the rules the caller actually sent to the datapath, and it is an argument
// because the caller is the only party that knows.** Walking the session's whole FAR list
// would be right for an establishment — everything the session has was just pushed — and
// wrong for a modification, which pushes only the rules the SMF restated. For every other FAR
// in that session the record would be rewritten from what this element *intends*, not from
// what the datapath was last told, and the two are only the same when nothing has gone wrong.
//
// The direction of that error is why this is an argument. A FAR the datapath
// refused, or one whose push was lost, is duplicating what it was duplicating before — and the
// record would then claim the tasking's intended value for it. Where the intent is "not
// duplicating", the next pass compares the record against the tasking, finds them equal, and
// concludes there is nothing to do: copies keep being made for a session no warrant covers,
// nothing in the element can turn them off, and the element's own account says duplication is
// off. That is over-collection recorded as compliance, the one direction the record must never
// be wrong in, and the signature makes it unstateable rather than remembered.
//
// **The request for a pass still considers the whole session**, and that is not an oversight in
// the other direction. What the record answers is "what does the datapath hold"; what the
// request answers is "might the right answer have changed here". A modification that restates
// one FAR can change what a criterion matches for the session's others — a UE address moves, a
// PDR is replaced — so a pass is wanted whenever the session's FARs and the record disagree,
// including for FARs this call did not push.
// confirmed says whether the datapath acknowledged the FAR writes. When it did not, the rules
// are recorded as *not* duplicating whatever this element meant to write -- the same rule
// farsAttempted follows, and for the same reason: a record that agrees with the tasking is
// one every later pass skips, so an unearned record is what turns an uncertain write into
// an interception that never runs.
func (e *ccEnabler) sessionProgrammed(s *PFCPSession, pushed []far, confirmed bool) {
	if e == nil || s == nil {
		return
	}

	e.mu.Lock()
	e.writes++
	stamp := e.writes
	for i := range pushed {
		ref := farRef{seid: s.localSEID, farID: pushed[i].farID}
		e.programmed[ref] = programmedFAR{
			duplicating: confirmed && pushed[i].liDuplicate,
			written:     stamp,
		}
		if pushed[i].liDuplicate {
			e.everDuplicated[ref] = true
		}
	}
	// Duplicating, because a copy running under tasking that may since have been
	// withdrawn is the thing that must be re-derived. Changed, because a
	// modification that took a session *out* of scope has to survive a pass holding
	// the older view just as much.
	//
	// Over the session rather than over pushed, per the note above: a FAR nobody pushed can
	// still be one whose answer this modification changed.
	// **And a FAR this element has ever turned on is notable whatever the record says.**
	//
	// Same reason the pass does not trust the record in the "off" direction: in the diverged
	// state both the record and the session's own flag read false, so neither of the two tests
	// below fires, no pass is requested, and the remedy in transact never gets the chance to run.
	// Distrusting the record when deciding what to *do* while trusting it when deciding whether
	// to *look* leaves the fix half-wired — it works only when some other event happens to ask
	// for a pass.
	//
	// The diverged state is reached by freezing the datapath so a withdrawal's push times out,
	// and without this the subscriber stays copied under no authority until some unrelated
	// event happens along.
	//
	// Bounded by the same small set: one pass request per session event touching a FAR that has
	// ever been intercepted, which is what a re-derivation is cheap enough for.
	notable := false
	for i := range s.fars {
		ref := farRef{seid: s.localSEID, farID: s.fars[i].farID}
		if s.fars[i].liDuplicate || e.programmed[ref].duplicating != s.fars[i].liDuplicate ||
			e.everDuplicated[ref] {
			notable = true

			break
		}
	}
	e.mu.Unlock()

	// This session's rules have changed, so an attribution answer computed from the
	// previous ones is wrong for it — and for it alone, which is why this is a delete
	// rather than a move of the epoch.
	e.forgetCovered(s.localSEID)

	if notable {
		e.request()
	}
}

// sessionForgotten drops what this element recorded about a session's FARs,
// because the session is gone and no re-derivation will ever mention it again.
//
// Without this the record only shrinks inside transact, which rebuilds it from
// the live sessions — and transact only runs when something asks for it. Nothing
// asks on the strength of an untasked session: sessionProgrammed requests a pass
// only when a FAR is duplicating or has stopped duplicating, which for a session
// no task covers is never. So an element with LI configured and its tasking
// stable — one long-lived warrant, or a target provisioned and not yet active,
// both ordinary — accumulates an entry per FAR per session for every subscriber
// that has ever attached, and reclaims none of it. The element keeps intercepting
// correctly the entire time, which is why nothing about it looks like a fault
// until the process dies and takes every warrant it holds with it.
//
// Keyed off the session's own FARs rather than scanning the record for a matching
// SEID: the same walk sessionProgrammed does to write them, so teardown costs
// what establishment did and not a pass over every session the element holds.
//
// The entries are only a record of what the datapath was last told. Dropping them
// for a session that no longer exists cannot lose an instruction, because there
// is nothing left to instruct.
//
// **It moves the write counter, and that is what makes the deletion visible to a pass
// already running.** A pass takes `mark` from `writes` at its start, rebuilds `fresh` from
// the sessions it walked, and at the end carries over any entry stamped past the mark. A
// deletion that did not move the counter would be invisible to the carry-over: the
// pass's own `fresh` map would still hold the entries it had planned from the session before
// it went, `e.programmed = fresh` would put them back, and the push could re-add the FAR to the
// datapath *after* the delete — duplication reinstated on a session that has been torn
// down, recorded as programmed, with nothing left to turn it off.
func (e *ccEnabler) sessionForgotten(s *PFCPSession) {
	if e == nil || s == nil {
		return
	}

	e.forgetCovered(s.localSEID)

	e.mu.Lock()
	defer e.mu.Unlock()

	// The same stamp sessionProgrammed takes, for the same reason: a pass holding an older
	// view must lose to a writer that has seen the session's real state — and a deletion is
	// the most authoritative statement about a session there is.
	e.writes++
	e.forgotten[s.localSEID] = e.writes

	for i := range s.fars {
		ref := farRef{seid: s.localSEID, farID: s.fars[i].farID}
		delete(e.programmed, ref)
		// And the ever-duplicated set, which is a record of what was programmed and is bound by
		// the same requirement. It is only ever consulted while walking the FARs of a live
		// session, so an entry whose session is gone can never be read again — see
		// everDuplicated.
		delete(e.everDuplicated, ref)
	}
}

// farsRemoved drops what this element recorded about FARs a session modification removed,
// while the session itself carried on.
//
// Nothing else reclaims them. sessionProgrammed walks the session's *remaining* FARs, so it
// never sees the one that went; sessionForgotten walks the same list at teardown, so the
// entry outlives the session that owned it; and transact rebuilds the record from live
// sessions, but only when something asks for a pass — and a removal that leaves nothing
// duplicating asks for none.
//
// So the record grows with every FAR any subscriber's session has ever had removed, and an
// element whose tasking is stable — one long-lived warrant, which is the ordinary case —
// reclaims none of it. That is the same leak sessionForgotten exists to prevent, reached
// through a different door, and it has the same shape: the element keeps intercepting
// correctly the whole time, so nothing looks like a fault until the process dies and takes
// every warrant it holds with it.
//
// **What the stale entry does not do is suppress a later interception**, and the distinction
// is worth stating because it is the first thing this looks like. Every path that creates a
// FAR pushes it to the datapath unconditionally — the record is consulted only to decide
// what a re-derivation needs to *change* — so a FAR re-created under the same identifier is
// programmed from the tasking either way. This is a leak, not a missed copy.
//
// It moves the write counter for the reason sessionForgotten documents: a deletion is
// invisible to transact's carry-over otherwise, and a pass planned from the pre-modification
// FAR list would put the entry straight back. That carry-over is not the only thing the
// stamp buys — a pass holding the older FAR list may be about to push the removed FAR, and
// the datapath's modify path programs what it is given, so it would re-create a rule the SMF
// has deleted.
func (e *ccEnabler) farsRemoved(seid uint64, removed []far) {
	if e == nil || len(removed) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.writes++

	for i := range removed {
		ref := farRef{seid: seid, farID: removed[i].farID}
		delete(e.programmed, ref)
		delete(e.everDuplicated, ref)
		e.forgottenFARs[ref] = e.writes
	}
}

// abandonedDuplication reports that this element gave up on a session while the datapath may
// still be duplicating its traffic.
//
// It exists for the paths where recording is not a remedy. A record is only ever read by a
// re-derivation walking the sessions this element holds, so a record written for a session that
// is being abandoned — an establishment the datapath refused, an association tearing down — can
// never be read again. On those paths the only real remedies are to remove the rules from the
// datapath, which the caller attempts, and to say so when that fails, which is this.
//
// Reported as a refused duplication change because that is what it is: the element instructed the
// datapath to stop duplicating and the datapath did not accept it. Only when a rule that may be
// duplicating is involved — an ordinary failed delete of a session no warrant covered is the
// datapath's business, not the ADMF's. Which rules those are is mayBeDuplicating's answer.
//
// Called before the session is forgotten, because forgetting it drops the record that answer reads.
func (e *ccEnabler) abandonedDuplication(seid uint64, fars []far, why string) {
	e.reportAbandoned(seid, fars, "the datapath did not accept the removal of a duplication rule", why)
}

// removalUnconfirmed is abandonedDuplication's report for a removal the datapath did not confirm,
// rather than one it refused: a batch that did not finish, or one that finished with a FAR delete
// whose RPC did not complete, which SendMsgToUPF answers as accepted. The rules may still be in
// the datapath, and the session is about to be forgotten either way, so the remedy is the same and
// only what happened differs. Saying "did not accept" for a removal that was never answered would
// send an operator looking for a refusal that never occurred.
func (e *ccEnabler) removalUnconfirmed(seid uint64, fars []far, why string) {
	e.reportAbandoned(seid, fars,
		"the datapath did not confirm the removal of a duplication rule", why)
}

// removalMayLeaveDuplication is mayBeDuplicating for a caller that holds no enabler when LI is
// off: whether an unconfirmed removal of these FARs may have left duplication running.
func (e *ccEnabler) removalMayLeaveDuplication(seid uint64, fars []far) bool {
	return e != nil && e.mayBeDuplicating(seid, fars)
}

func (e *ccEnabler) reportAbandoned(seid uint64, fars []far, what, why string) {
	if e == nil || e.report == nil || !e.mayBeDuplicating(seid, fars) {
		return
	}

	e.report(x1.NEIssueDuplicationRefused,
		what+" for a session this element is no longer tracking, so content may still be "+
			"copied with nothing able to stop it: "+why)
}

// mayBeDuplicating says whether any of a session's FARs may be duplicating in the datapath.
//
// **The session's own liDuplicate is not enough to answer it.** That flag is what the session
// path last derived, and the re-derivation deliberately writes the record, not the session
// store: a warrant activated after the UE attached is turned on by a pass, so the datapath is
// duplicating while every stored FAR still says it is not. Gating on the flag alone would make
// a refused teardown delete of exactly that session — the ordinary operational case — silent.
//
// So the record answers too, and everDuplicated rather than the current record value, for the
// reason everDuplicated exists: a record that wrongly says "off" is the unrecoverable direction.
// Every write of a duplicating record also writes everDuplicated, so it covers those. A rule this
// element once turned on and has since turned off costs, at worst, a report that says content
// *may* still be copied.
func (e *ccEnabler) mayBeDuplicating(seid uint64, fars []far) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range fars {
		if fars[i].liDuplicate || e.everDuplicated[farRef{seid: seid, farID: fars[i].farID}] {
			return true
		}
	}

	return false
}

// farsAttempted records rules whose outcome this element could **not** determine — the
// refused-modification branches, where a refusal does not mean the datapath is untouched and
// the rollback that follows it is itself unconfirmed.
//
// It records them as not duplicating, and that is the point of the function. transact
// skips a FAR whose record already agrees with the tasking unless the answer is "off"
// (see recordIsEnough), so recording the *intended* value for a write that may never have
// landed makes every later pass skip it — for the life of the session. The warrant then
// produces nothing, permanently, while this element answers that it is healthy: the mirror
// of the over-collection defect everDuplicated exists to prevent, reached the same way, by
// trusting a record that was never earned.
//
// Recording "off" costs one redundant write per pass where the rules did land — the trade
// everDuplicated already makes — and buys two things: the record disagrees with the
// tasking, so the next re-derivation re-pushes; and taskFaults counts the rule as not
// duplicating, so an interrogation answers duplicationNotProgrammed rather than claiming
// the task is faultless.
//
// everDuplicated is still set from what was *intended*, so the over-collection direction is
// untouched: a rule this element tried to turn on is never trusted-to-be-off again, which
// is exactly the case where the datapath may have applied the write after all.
func (e *ccEnabler) farsAttempted(seid uint64, fars []far) {
	e.recordFARs(seid, fars, false)
	e.reportUnconfirmedDuplication(fars)
}

// reportUnconfirmedDuplication tells the provisioning function that rules for a task this
// element accepted may not have been applied.
//
// Correcting is not enough on its own. The record above makes the next pass rewrite the
// rules, which restores the interception — but it says nothing about the interval before
// that pass runs, in which an accepted warrant produced no product. An agency receiving
// nothing cannot tell that from a subject who is not communicating, and this element is the
// only party that knows the write was unconfirmed. If it does not say so, the interval has
// no representation anywhere.
//
// Raised here rather than in the handler so that a path added later inherits it by calling
// farsAttempted, instead of having to remember to report as well as record: a remedy placed
// on one path is a remedy whose presence depends on which path runs.
//
// Only where duplication is involved. An ordinary rule this element could not confirm is
// the datapath's business; it becomes the ADMF's only when a warrant depends on it.
func (e *ccEnabler) reportUnconfirmedDuplication(fars []far) {
	if e == nil || e.report == nil {
		return
	}

	for i := range fars {
		if !fars[i].liDuplicate {
			continue
		}

		e.report(x1.NEIssueDuplicationRefused,
			"the datapath did not confirm a duplication rule for an accepted interception "+
				"task, so this element cannot say whether the interception is running and it "+
				"may be producing nothing until the next re-derivation rewrites it")

		return
	}
}

// recordFARs is the body of both. confirmed says whether the datapath's acceptance is
// known; when it is not, the record must not claim the rule is duplicating, whatever this
// element meant to write.
func (e *ccEnabler) recordFARs(seid uint64, fars []far, confirmed bool) {
	if e == nil || len(fars) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.writes++
	stamp := e.writes

	for i := range fars {
		ref := farRef{seid: seid, farID: fars[i].farID}
		e.programmed[ref] = programmedFAR{
			duplicating: confirmed && fars[i].liDuplicate,
			written:     stamp,
		}
		if fars[i].liDuplicate {
			e.everDuplicated[ref] = true
		}
	}
}

// retask asks for duplication to be re-derived for every session this element
// holds, and returns as soon as the request is registered. It is called when the
// tasking changes — a task activated, modified or withdrawn — which is when a
// session's rules are correct but the answer about them is not.
//
// The work happens on the enabler's worker, so this does not block an X1 request
// behind a walk of every session. Where the caller's own report depends on the
// datapath having actually been reprogrammed, use retaskAndWait: this returning
// does not mean the datapath is programmed.
func (e *ccEnabler) retask() {
	if e == nil {
		return
	}

	// Before the request, not after: a transaction that began first would otherwise
	// derive duplication from the previous snapshot while the request that prompted it
	// was already counted as served.
	e.reparse()
	e.request()
}

// retaskAndWait asks for the same re-derivation and waits for it to be programmed.
//
// It exists for the deactivation path, where the element reports that interception
// has stopped: that report is the outcome being reported, so it must follow the
// datapath having actually stopped duplicating rather than merely a request to.
func (e *ccEnabler) retaskAndWait() {
	if e == nil {
		return
	}

	e.reparse()
	e.await(e.request())
}

// transact is the re-derivation itself: read the current tasking, derive the rules
// it implies, program the difference, and record what was programmed. The worker
// runs one at a time, which is what makes those four steps atomic with respect to
// another pass.
//
// It does not write to the session store. It runs while the PFCP goroutine may be
// part-way through its own read-modify-write of the same session, and writing back a
// session read before that started would discard the SMF's update — corrupting a
// subscriber's own forwarding to serve an interception, which is the one thing this
// must never do. The datapath's modify path takes only the rules being changed, so a
// FAR can be reprogrammed without restating the session.
//
// The store therefore does not learn about duplication enabled this way. It does
// not need to: every path that rebuilds a FAR re-derives duplication from the
// tasking as it goes.
func (e *ccEnabler) transact() {
	e.mu.Lock()
	sources := append([]SessionsStore(nil), e.sources...)
	// Taken before any session is read, so an entry written later belongs to a
	// session this pass cannot have seen — sessionProgrammed writes only once the
	// session is in the store.
	mark := e.writes
	e.mu.Unlock()

	// Parsed once: the criteria are the same for every session, and re-parsing them
	// per session would put string handling in the loop.
	criteria := e.criteriaOf(e.tasks.Snapshot())

	// Rebuilt rather than amended, so FARs of sessions that have gone away drop out
	// instead of accumulating.
	fresh := make(map[farRef]programmedFAR)

	for _, src := range sources {
		for _, sess := range src.GetAllSessions() {
			want := wantedFARs(criteria, sess)

			var changed []far
			// What the record said before this pass touched it, for the FARs this pass
			// is about to program. If the datapath refuses them, the record has to go
			// back to describing what the datapath actually holds — not forward to
			// what was intended, and not to nothing: an entry deleted for a rule the
			// element was *turning off* would leave the next pass finding nothing to
			// do while the datapath went on duplicating.
			type priorEntry struct {
				ref  farRef
				held bool
				was  programmedFAR
			}

			var prior []priorEntry

			e.mu.Lock()
			for i := range sess.fars {
				ref := farRef{seid: sess.localSEID, farID: sess.fars[i].farID}
				if cur, held := e.programmed[ref]; held && cur.written > mark {
					// The session path re-derived this FAR from rules this pass did not
					// read, and programmed the datapath accordingly. Leave both alone: the
					// FAR body held here is the pre-modification one, so pushing it would
					// restate the SMF's own forwarding from a copy that has been replaced.
					// The carry-over at the end keeps the record; the request
					// sessionProgrammed made brings a pass that has read the new rules.
					continue
				}
				fresh[ref] = programmedFAR{duplicating: want[ref.farID], written: mark}
				if want[ref.farID] {
					// Told to duplicate, so it joins the set that is never trusted-to-be-off
					// again. Recorded before the push: a push that is refused leaves the
					// datapath as it was, and a FAR wrongly *in* this set costs one redundant
					// write per pass, while one wrongly out of it costs unbounded
					// over-collection.
					e.everDuplicated[ref] = true
				}
				// A record that agrees is normally enough to skip — that is what the record is
				// for. **Except when the answer is "off" and this element has ever turned this
				// FAR on**, in which case the record is exactly what cannot be trusted: if it
				// wrongly says "off" the datapath keeps duplicating and no later pass will ever
				// look again. See everDuplicated. The cost is one redundant write per pass per
				// FAR that has been turned off, which is a small and shrinking set; the harm it
				// removes is a subscriber's traffic being copied under no authority at all.
				recordIsEnough := want[ref.farID] || !e.everDuplicated[ref]
				if e.programmed[ref].duplicating == want[ref.farID] && recordIsEnough {
					continue
				}
				f := sess.fars[i]
				f.liDuplicate = want[ref.farID]
				changed = append(changed, f)
				was, held := e.programmed[ref]
				prior = append(prior, priorEntry{ref: ref, held: held, was: was})
			}
			e.mu.Unlock()

			// Released once the push's answer is recorded in fresh; a no-op unless this
			// session has something to push.
			unlockSession := func() {}

			if len(changed) > 0 && e.push != nil {
				// **Re-read under the session's stripe, because the plan above is not.**
				// e.mu is dropped after planning — the push is a round trip to the
				// datapath, and holding e.mu across it would serialise every session
				// handler behind one gRPC call — so the session path can complete its own
				// read-modify-write of one of these FARs after the plan: program the
				// datapath with the new body and record the write. Pushing the planned
				// body after that would restate the SMF's forwarding from a snapshot that
				// has been replaced: the interception plane corrupting the subscriber's
				// own forwarding, which is the one thing this function's contract says it
				// must never do.
				//
				// The stripe makes the re-read, the push and the recording of its answer
				// one step with respect to the session handlers, which hold the same
				// stripe from before their first datapath write until they return. So
				// either the handler stamped first, and the test below — the one the
				// plan applies, an entry written past this pass's mark — drops
				// those FARs, leaving the datapath with the handler's body; or this push
				// reaches the datapath before the handler's does, and the handler's write
				// lands last. The carry-over at the end keeps the handler's record; the
				// request its write made brings a pass that has read the new rules.
				//
				// Sessions with nothing to push take no stripe.
				if beforeTransactPush != nil {
					beforeTransactPush()
				}

				unlockSession = e.lockSession(sess.localSEID)

				e.mu.Lock()
				// **The session itself, before any of its FARs.** This extends the
				// per-FAR test below to the one thing that test cannot express: a session
				// *deleted* mid-pass has no FARs left to compare stamps against, so
				// nothing in the per-FAR loop would notice, and the push would re-add a
				// FAR to the datapath after the delete — duplication reinstated on a
				// session that has been torn down, with nothing left to turn it off.
				//
				// sessionForgotten stamps the deletion into the same counter the writes
				// use, so "the session went after this pass started" is the same
				// comparison as "this FAR was written after this pass started".
				if gone, deleted := e.forgotten[sess.localSEID]; deleted && gone > mark {
					e.mu.Unlock()
					unlockSession()

					// Its entries must not be carried into the conclusion either: the
					// deletion already removed them from e.programmed, and fresh is this
					// pass's own map.
					for i := range sess.fars {
						delete(fresh, farRef{seid: sess.localSEID, farID: sess.fars[i].farID})
					}

					continue
				}

				kept := make([]far, 0, len(changed))
				keptPrior := make([]priorEntry, 0, len(prior))
				for i := range changed {
					ref := farRef{seid: sess.localSEID, farID: changed[i].farID}
					if cur, held := e.programmed[ref]; held && cur.written > mark {
						continue
					}
					// Removed while this pass ran. Pushing it would not merely restate a
					// stale body — the datapath's modify path programs what it is given,
					// so it would re-create a FAR the SMF has deleted, on the strength of
					// the interception plane wanting to duplicate it.
					if gone, removed := e.forgottenFARs[ref]; removed && gone > mark {
						delete(fresh, ref)

						continue
					}
					kept = append(kept, changed[i])
					keptPrior = append(keptPrior, prior[i])
				}
				e.mu.Unlock()

				changed, prior = kept, keptPrior
			}

			if len(changed) > 0 && e.push != nil {
				// Only the changed FARs: the datapath's modify path programs what it is
				// given, and restating the rest would rewrite rules it already has.
				cause, confirmed := e.push(PacketForwardingRules{}, PacketForwardingRules{fars: changed})
				switch {
				case cause != ie.CauseRequestAccepted:
					// The datapath answered, and the answer was that none of this was
					// programmed. It is one cause for the whole write, so the whole
					// write is what is refused. fresh is this pass's own map and no
					// other goroutine reads it, so no lock is needed; the carry-over
					// below still lets a newer write win over this restoration, which
					// is what keeps a session modified mid-pass authoritative.
					for _, p := range prior {
						if p.held {
							fresh[p.ref] = p.was
						} else {
							delete(fresh, p.ref)
						}
					}
					if e.report != nil {
						e.report(x1.NEIssueDuplicationRefused,
							"the datapath refused a duplication rule for an accepted interception task")
					}
				case !confirmed:
					// The datapath did not answer for at least one of these writes. Not
					// a refusal, so the previous values are not restored -- they are no
					// more true than what was written. The record is set to *not
					// duplicating* instead, which is the only claim that is safe in both
					// directions: the next pass finds it disagreeing with the tasking and
					// rewrites the rule, and taskFaults counts it as not duplicating so
					// an interrogation answers with a fault rather than claiming the
					// warrant is being served. everDuplicated is untouched, so the
					// over-collection direction keeps its protection.
					//
					// The same trade farsAttempted documents: one redundant write per
					// pass where the rules did land, against an accepted warrant silently
					// producing nothing.
					for _, p := range prior {
						cur, held := fresh[p.ref]
						if !held {
							continue
						}

						cur.duplicating = false
						fresh[p.ref] = cur
					}
					if e.report != nil {
						e.report(x1.NEIssueDuplicationRefused,
							"the datapath did not confirm a duplication rule for an accepted "+
								"interception task, so this element cannot say whether the "+
								"interception is running until a later re-derivation rewrites it")
					}
				}
			}

			unlockSession()
		}
	}

	e.mu.Lock()
	// An entry written while this pass ran wins over its conclusion, whether or not
	// it reached one: a session established mid-pass is invisible to it, and a
	// session modified mid-pass was read in a state that has been replaced. Entries
	// older than the mark and absent from fresh belong to sessions that have gone
	// away and drop, which is what rebuilding the map was for.
	for ref, cur := range e.programmed {
		if cur.written > mark {
			fresh[ref] = cur
		}
	}
	// And the deletions, which the loop above cannot express: an entry that is *gone* from
	// e.programmed is not there to be carried over, so a pass that planned one from a view
	// predating the deletion would write its own copy straight back.
	//
	// The push block already does this for the sessions and FARs it pushed. It is not
	// enough on its own: that block runs only when this pass had something to push, and a
	// pass that concluded nothing was to be changed still planned an entry for every FAR it
	// walked. That is the ordinary case for an element whose tasking is stable, which is
	// also the case sessionForgotten exists to reclaim.
	// **Against the entry's own stamp, not the pass's mark.** Comparing to `mark` answers "was
	// this forgotten after the pass began", which discards an entry written *after* the
	// forgetting too — and a FAR re-created under the same identifier is exactly that. A path
	// switch does it routinely: the modification removes FAR n and stamps it forgotten, then
	// re-creates FAR n and records it duplicating. Both stamps are past `mark`, so the sweep
	// dropped a record describing a live, duplicating rule.
	//
	// A missing entry reads as "not duplicating", so until the next pass this element reports
	// TaskIssueDuplicationNotProgrammed for a warrant it is in fact serving. The "off" direction
	// is covered by everDuplicated and the next pass re-pushes redundantly, which is why this is
	// a wrong answer rather than lost interception — but the answer is wrong, and it is the
	// answer the triggering function acts on.
	for ref, cur := range fresh {
		if gone, deleted := e.forgotten[ref.seid]; deleted && gone > cur.written {
			delete(fresh, ref)

			continue
		}
		if gone, removed := e.forgottenFARs[ref]; removed && gone > cur.written {
			delete(fresh, ref)
		}
	}
	e.programmed = fresh

	// And the ever-duplicated set, bounded against the record that was just rebuilt.
	//
	// sessionForgotten and farsRemoved reclaim an entry the moment its FAR goes, which is the
	// common case, but they cannot be the whole answer: this pass writes the set directly rather
	// than through `fresh`, so a pass that planned from a snapshot taken before a deletion
	// re-adds entries for FARs that have since gone — after the reclamation ran. Sweeping here
	// closes that, because `fresh` was just rebuilt from the sessions that actually exist, so an
	// entry it does not name is one no walk of live sessions can ever reach.
	//
	// Not a substitute for the reclamation either: the sweep only runs when a pass does, and a
	// steadily-tasked element is not guaranteed to run one.
	for ref := range e.everDuplicated {
		if _, live := fresh[ref]; !live {
			delete(e.everDuplicated, ref)
		}
	}

	// The deletions that predate this pass: it read the world after them, so no pass will
	// ever need to be told about them again. The ones stamped *during* this pass were acted
	// on above and are dropped by the next pass, whose mark is past them — the worker is
	// serial, so no pass with an older mark can still be running.
	//
	// Reclaimed at all because a map that grows with every released session and is never
	// emptied would be precisely the leak sessionForgotten exists to prevent, reintroduced by
	// its own bookkeeping.
	for seid, gone := range e.forgotten {
		if gone <= mark {
			delete(e.forgotten, seid)
		}
	}
	for ref, gone := range e.forgottenFARs {
		if gone <= mark {
			delete(e.forgottenFARs, ref)
		}
	}
	e.mu.Unlock()
}

// beforeTransactPush is called after a pass has planned a session's FAR bodies and
// before it takes the session's stripe to re-read them and push. Set only by tests; nil
// otherwise.
//
// It marks the last point at which a session handler can run to completion without
// meeting the pass: from the stripe onwards, the re-read, the push and the recording of
// its answer are one step with respect to the handlers (see sessionLocks). A handler run
// from this hook has therefore finished — datapath write, PutSession and stamp — before
// the pass re-reads, which is the interleaving the re-read exists for; one that starts
// after the pass has taken the stripe waits for it, and its write reaches the datapath
// after the pass's. See TestATaskingPassDoesNotRestateAStaleForwardingBody and
// TestATaskingPushAndAModificationDoNotInterleave.
var beforeTransactPush func()

// lostContent reports a copy this element made and did not deliver, through the condition
// every other content loss uses. Nil-safe in both directions: an enabler without a reporter
// says nothing, which is the certificate-less/no-ADMF case.
func (e *ccEnabler) lostContent() {
	if e == nil || e.report == nil {
		return
	}
	e.report(x1.NEIssueX3DeliveryLost,
		"a fragment of an authorised datagram was discarded because its first fragment "+
			"had not been seen")
}

// sessionFor returns the session with the given SEID from whichever association
// holds it.
func (e *ccEnabler) sessionFor(seid uint64) (PFCPSession, bool) {
	e.mu.Lock()
	sources := append([]SessionsStore(nil), e.sources...)
	e.mu.Unlock()

	for _, src := range sources {
		if sess, ok := src.GetSession(seid); ok {
			return sess, true
		}
	}

	return PFCPSession{}, false
}

// coveredTask is a task that covers a session, with the criteria resolved against
// that session so each copy can be decided without resolving them again.
type coveredTask struct {
	task   types.InterceptTask
	filter copyFilter
}

// tasksCovering returns the active tasks whose detection criteria select traffic in
// the session the datapath tagged a copy with, in XID order.
//
// **The answer is memoised per session, and stamped rather than indexed.** An index maintained incrementally has to
// be invalidated at every site that changes tasking or session rules, and a site that
// forgets goes stale silently — product attributed to the wrong warrant, or dropped as
// untasked, neither visible from outside this element. An entry stamped with the
// tasking epoch it was computed under cannot: a forgotten bump invalidates *everything*
// rather than one thing wrongly, which fails toward recomputation.
//
// Two writers, and they invalidate different amounts because they affect different
// amounts. A change in tasking moves the epoch, which retires every session's answer.
// A change in one session's own rules, or that session going away, drops that session's
// entry — reparse would be the wrong instrument there, since the tasking has not moved.
//
// Why memoise at all: computing the answer from scratch takes a full store snapshot
// with a deep clone of every task's four slices, a parse of every criterion of every
// task, a resolve over every PDR, then filterFor parsing the same criteria a second
// time — and for a PDR criterion an IE parse and a throwaway address pool. That is
// about 95 allocations per copy for three warrants against one session, where framing
// is one allocation and a memcpy and delivery amortises across a 32-PDU batch. This is
// the one path in this element whose cost is per packet, in front of a socket queue holding ten datagrams by default whose
// overflow is intercept product nothing downstream can recover.
func (e *ccEnabler) tasksCovering(seid uint64) []coveredTask {
	if e == nil || e.tasks == nil {
		return nil
	}

	// The epoch is read first. Reading it after computing would let a tasking change
	// that landed during the computation be stamped as though it had been accounted
	// for, and nothing would recompute it.
	epoch := e.epoch.Load()

	e.coveredMu.Lock()
	held, ok := e.covered[seid]
	e.coveredMu.Unlock()

	if ok && held.epoch == epoch {
		return held.tasks
	}

	out := e.resolveCovering(seid)

	e.coveredMu.Lock()
	defer e.coveredMu.Unlock()

	// **Whoever got there first wins, and the loser discards its own answer.** Resolution runs
	// unlocked — it has to, since it walks every session — so two callers missing on the same
	// session at the same time would both compute one. The shipper cannot do that today, since
	// a session is framed by one worker (frameQueueFor); the check keeps it so if another caller
	// comes. The answers are equivalent, but they are
	// not interchangeable: resolveCovering allocates the *fragment memo* the filter carries, so
	// two answers are two memos. Last-writer-wins left the other worker holding a filter whose
	// memo nothing else shares, and a datagram whose initial fragment was classified into one
	// memo and whose later fragments were looked up in the other loses its tail — dropped, and
	// reported as X3 delivery loss, for content the criterion did select.
	if held, ok := e.covered[seid]; ok && held.epoch == epoch {
		return held.tasks
	}

	// Only where the session still exists. A session that has gone away is dropped by
	// forgetCovered, and storing an answer for it here would put back the entry that
	// just removed — an entry nothing would ever remove again, since the events that
	// prune this map have both already happened for that session.
	if out != nil {
		e.covered[seid] = coveredEntry{epoch: epoch, tasks: out}
	}

	return out
}

// resolveCovering is tasksCovering's answer computed from scratch: which of the live
// content tasks' criteria select traffic in this session, in XID order.
//
// The order is the snapshot's, which is ordered by XID — the caller picks one task
// when several cover a session, and picking a different one per packet would split a
// session's product across the covering warrants so that no agency gets a whole
// stream.
func (e *ccEnabler) resolveCovering(seid uint64) []coveredTask {
	sess, ok := e.sessionFor(seid)
	if !ok {
		// The session went away between the datapath duplicating the packet and this
		// running. Nothing to attribute the copy to.
		return nil
	}

	parsed := e.parsed.Load()
	if parsed == nil {
		return nil
	}

	one := []PFCPSession{sess}

	var out []coveredTask
	for _, pt := range *parsed {
		if len(selection(pt.criteria, one)) == 0 {
			continue
		}
		out = append(out, coveredTask{
			task: pt.task,
			// The reporter is this element's, so the memo built inside can report a
			// fragment it had to discard through the same channel every other content
			// loss uses.
			filter: filterFrom(pt.criteria, sess, e.lostContent),
		})
	}

	return out
}

// taskFaults answers what is wrong with one task's interception, now, for the triggering
// function that asks — see x1.WithTaskFaults.
//
// **Computed, never stored, and the three conditions it reports are the ones a CC-POI can
// re-observe.** A refusal from the datapath is an event; whether this element is duplicating
// what a task requires is a state, and it is the state that matters — it is equally true after a
// refusal, after a push that was lost, and after a FAR that was never programmed at all. So the
// question asked here is not "did something fail" but "does the record of what the datapath was
// told match what this task's criteria select":
//
//   - Traffic this task selects that the datapath is not duplicating. The record is only an
//     account of what the datapath was last told, which is exactly the right authority for this
//     question: an entry saying `duplicating: false`, or no entry at all, means no copy is being
//     made whatever the tasking says should happen.
//   - No session at all that this task's criteria select. A CC-POI's task is installed by a
//     triggering function against traffic that exists, so a task selecting nothing is an
//     interception producing nothing — and the answer says what was observed rather than
//     accusing anybody, because a session torn down and one never established look the same from
//     here.
//   - Sessions this task selects that a lower-XID task also selects. The shipping path attributes
//     each copy to the first covering task and filters it by that task's criteria alone, so this
//     task receives none of that session's content: the part the first task also wants is
//     delivered under it, and the part the first task's criteria exclude — the other direction of
//     a tunnel-direction task, say — is not delivered at all. Only sessions this element has
//     stored are walked, so a session whose rules reach the datapath before it is stored is
//     answered for once it is stored, not before.
//
// **Reported against the task and nowhere else.** Both conditions are already reported at
// element scope, which is what a triggering function has had to work with: it learns that
// something at this point of interception is wrong and cannot tell which of the warrants it
// installed is affected. That attribution is the whole difference, and it is why `triggerFaulty`
// could not be raised for one warrant.
//
// It reads the parsed snapshot rather than re-parsing, so the criteria are the same ones the
// shipping path resolves copies against — an answer computed from a second parse could differ
// from the interception it describes.
//
// A task the snapshot does not carry answers nothing: either it requires no content, in which
// case this element has nothing to say about it, or its criteria do not parse, which activation
// already refuses.
// faultEntry is one memoised answer to "what is wrong with this task", with the two stamps that
// decide whether it is still true.
type faultEntry struct {
	epoch  uint64
	writes uint64
	faults []x1.X1Error
}

// taskFaults answers what is currently wrong with one task's content interception.
//
// **Memoised, because the X1 request goroutine is not where a walk of every session belongs.**
// li/x1 calls this once per task on *every* response that carries tasks — an activation, a
// modification, a deactivation, a status query alike — and the computation resolves every
// criterion against every session this element holds. Unmemoised, an element with twenty thousand
// sessions and ten tasks would resolve two hundred thousand criterion sets to answer one
// provisioning message, on the goroutine answering it.
//
// That is precisely the cost `run()`'s own doc-comment says the worker exists to keep off the
// request path: "an X1 request's latency would become a function of session count on the element
// holding the most sessions".
//
// The answer is still *determined when asked* rather than pushed or accumulated: the stamps are
// the tasking epoch and the record's write counter, and every event that could change the answer
// moves one of them. What is avoided is recomputing an answer nothing has invalidated.
func (e *ccEnabler) taskFaults(xid types.XID) []x1.X1Error {
	if e == nil {
		return nil
	}

	epoch := e.epoch.Load()

	e.mu.Lock()
	writes := e.writes
	e.mu.Unlock()

	e.faultsMu.Lock()
	if held, ok := e.faultsMemo[xid]; ok && held.epoch == epoch && held.writes == writes {
		e.faultsMu.Unlock()

		return held.faults
	}
	e.faultsMu.Unlock()

	faults := e.resolveTaskFaults(xid)

	e.faultsMu.Lock()
	// Entries from a superseded epoch can never be returned again, so they are dropped rather
	// than left to accumulate one per task this element has ever held.
	for held := range e.faultsMemo {
		if e.faultsMemo[held].epoch != epoch {
			delete(e.faultsMemo, held)
		}
	}

	e.faultsMemo[xid] = faultEntry{epoch: epoch, writes: writes, faults: faults}
	e.faultsMu.Unlock()

	return faults
}

func (e *ccEnabler) resolveTaskFaults(xid types.XID) []x1.X1Error {
	snapshot := e.parsed.Load()
	if snapshot == nil {
		return nil
	}

	// The tasks ahead of this one in the snapshot's XID order are the ones the shipping path
	// would deliver a session's content under instead — it takes the first covering task.
	var (
		criteria []criterion
		ahead    [][]criterion
	)
	for i := range *snapshot {
		if (*snapshot)[i].task.XID == xid {
			criteria = (*snapshot)[i].criteria

			break
		}
		ahead = append(ahead, (*snapshot)[i].criteria)
	}
	if len(criteria) == 0 {
		return nil
	}

	e.mu.Lock()
	sources := append([]SessionsStore(nil), e.sources...)
	e.mu.Unlock()

	selecting, undup, elsewhere := 0, 0, 0
	for _, src := range sources {
		if src == nil {
			continue
		}
		for _, sess := range src.GetAllSessions() {
			want := wantedFARs([][]criterion{criteria}, sess)
			if len(want) == 0 {
				continue
			}
			selecting++
			if anySelects(ahead, sess) {
				elsewhere++
			}

			e.mu.Lock()
			for farID := range want {
				if p, held := e.programmed[farRef{seid: sess.localSEID, farID: farID}]; !held || !p.duplicating {
					undup++
				}
			}
			e.mu.Unlock()
		}
	}

	if selecting == 0 {
		return []x1.X1Error{x1.TaskFault(x1.TaskIssueNoTrafficSelected,
			"no session this element holds carries traffic this task selects, so this "+
				"interception is producing nothing")}
	}

	// Both of the remaining conditions can hold at once, and each is its own state with its own
	// retraction, so both are answered.
	var faults []x1.X1Error
	if undup > 0 {
		faults = append(faults, x1.TaskFault(x1.TaskIssueDuplicationNotProgrammed, fmt.Sprintf(
			"the datapath is not duplicating %d forwarding rule(s) this task requires, across "+
				"%d session(s) it selects", undup, selecting)))
	}
	if elsewhere > 0 {
		faults = append(faults, x1.TaskFault(x1.TaskIssueContentUnderAnotherTask, fmt.Sprintf(
			"%d session(s) this task selects are also selected by a lower-XID task; their content "+
				"is delivered under that task, or not at all where that task's own criteria "+
				"exclude it", elsewhere)))
	}

	return faults
}

// anySelects reports whether any of the criteria sets selects traffic in the session.
//
// It is what takes a task's content away: the shipping path attributes each copy to the first
// covering task in XID order and filters it by that task's criteria only, so of a session one of
// the tasks ahead also selects, this task receives nothing — what the task ahead also wants goes
// to it, and what its criteria exclude is dropped rather than tried against the next task. Asked per selected session of the task being answered for, so its
// cost is the tasks ahead of it per session — inside the walk of every session resolveTaskFaults
// already makes, and paid only when taskFaults' memo has been invalidated.
func anySelects(sets [][]criterion, sess PFCPSession) bool {
	one := []PFCPSession{sess}
	for _, set := range sets {
		if len(selection(set, one)) > 0 {
			return true
		}
	}

	return false
}
