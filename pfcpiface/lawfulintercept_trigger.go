// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// The UPF is a triggered CC-POI: it does not learn about warrants from the ADMF.
// The CC Triggering Function in the SMF tasks it over LI_T3, which TS 33.128
// clause 5.2.6 realises as ETSI TS 103 221-1 with the CC-TF in the role of the
// "ADMF" and this CC-POI in the role of the "NE". Each trigger names the PFCP
// session to intercept and carries what the datapath cannot know: the warrant XID
// to label content with, the correlation identifier that joins it to the SMF's
// signalling, and where to deliver it.
//
// The PFCP DUPL apply-action remains the mechanism that replicates packets;
// this interface supplies the identity of what is replicated. The
// two therefore arrive over different interfaces and can disagree — content whose
// session has no trigger must not be delivered, since it could only be labelled
// with an XID no mediation function can attribute.

// startTriggerListener binds the LI_T3 triggering interface and returns the task
// store the shipper matches duplicated content against.
//
// The bind is synchronous so a failure reaches the caller: a CC-POI that looks
// enabled but can never be tasked performs no interception, and the whole point
// of failing closed here is that this is not discoverable from the outside.
// Nothing is logged — that this NE can be tasked at all must not appear in a
// general operator log.
//
// The probes are the conditions this element can be asked about. They are the
// shipper's, since the knowledge is the shipper's, and are passed in rather than
// built here because this function owns the interface and not the delivery.
func startTriggerListener(cfg *LiConfig, serverTLS *tls.Config, reporter neIssueReporter,
	enabler *ccEnabler, ids *x2x3.Identity, onTasking func(),
	unreachable func(addr string) bool, probes ...x1.FaultProbe,
) (*store.Store, error) {
	// Parse the keepalive timers before anything is bound. Doing it afterwards would
	// leave a listener accepting and applying tasking into a store this function had
	// already abandoned by returning an error — an element that looks un-tasked to
	// its operator while quietly holding warrants.
	keepalive, err := keepaliveProcedure(cfg)
	if err != nil {
		return nil, err
	}
	// The X1 reporter, where one is configured: the keepalive procedure's reports and the
	// task-fault watcher need the reporter itself, not just its notifications.
	x1Reporter, _ := reporter.(*x1.Reporter)

	tasks := store.New()
	// setTasks rather than an assignment: it also parses what the store holds, so the
	// shipping path is not left falling back to the one criterion a bare store lookup
	// can answer until the first tasking change happens to rebuild it.
	enabler.setTasks(tasks)
	// A trigger naming a destination this POI has lost — after a restart, say — is
	// refused with 2040 by every element. RequireResolvableDIDs covers the remaining
	// case, a trigger ordering content that names no destination at all: this POI has
	// no configured X3 endpoint to serve it from, so it is refused with 3040. Accepting
	// either would mean duplicating a subject's traffic and discarding every copy while
	// the triggering function is told interception is running, which is exactly the
	// failure this refuses. The keepalive procedure reports its own
	// deactivation to the ADMF (Alert 10000, through the reporter passed to the server):
	// interception stopping is the safe outcome, but only if somebody is told it
	// happened.
	//
	// Deliberately *without* x1.WithConfiguredDestinations, which the AMF and SMF do wire.
	// A triggered POI takes its destinations from the function that triggered it and from
	// nowhere else: the capability requires that a network function "SHALL NOT synthesise a
	// missing task attribute from local state or configuration", and letting an operator
	// declare a destination here would give the UPF an X3 endpoint the CC-TF never
	// provisioned. Adding it for symmetry with the other two POIs would be a regression, not
	// a tidy-up.
	opts := []x1.Option{
		x1.WithADMF(cfg.TFID),
		x1.RequireResolvableDIDs(),
		// What this element can observe about itself, for the triggering function that asks
		// for its status. A triggered CC-POI is the one element whose product loss is
		// invisible everywhere else: a dropped copy produces no record for anybody to miss.
		x1.WithFaultProbes(probes...),
		// And the same knowledge at destination scope, for the triggering function that asks
		// about one destination rather than about this element. Without it the answer would be
		// activeAndWorking unconditionally, while this element reports the same endpoint
		// unreachable over ReportDestinationIssue — and a CC-POI is the element
		// where that matters most, since a dropped copy leaves no record anywhere else for
		// the discrepancy to show up in.
		x1.WithDestinationReachability(unreachable),
		// And at task scope, which is the one a triggering function can act on. The two above
		// tell it that something at this point of interception is wrong; this tells it which of
		// the warrants it installed is affected, which is what `triggerFaulty` needs and could
		// not be raised without.
		x1.WithTaskFaults(enabler.taskFaults),
		// One hook for the whole lifecycle: an activation, a modification and a
		// removal all change what this datapath must duplicate, and all of them are
		// answered the same way — by re-deriving duplication from the tasking that
		// remains. The triggering function may also change a task's products or its
		// criteria without touching its targets, and that change arrives through
		// this hook too.
		x1.OnTaskChange(func(prev, next *types.InterceptTask) {
			// Whatever duplication the tasking requires now, and only that.
			//
			// onTasking is the shipper's: which destinations are in use is a function of the
			// tasking, so this is the one event that can make a delivery client
			// unreferenced. Called on every branch, including the removal below.
			if onTasking != nil {
				defer onTasking()
			}
			if next != nil {
				enabler.retask()

				// A modification keeps the *task's* XID, and leaving the numbering alone
				// is right for every modification but one: the numbering is keyed by the
				// **delivery** XID, which here is the warrant's ProductID, and a relabel
				// moves it. The context under the superseded label is then stranded —
				// every copy from this point carries the new label, so nothing will
				// number under the old one again.
				//
				// Released per context, not per XID, and this is the case that shows why:
				// the triggering function relabels a warrant's triggers one at a time, so
				// while this one is being modified its sibling sessions are still
				// delivering under the old label. Forget would take their numbering with
				// it, which is the defect this granularity exists to avoid, reached from
				// the modification path instead of the removal path.
				if prev != nil && prev.DeliveryXID() != next.DeliveryXID() {
					var corr [x2x3.CorrelationIDLength]byte
					binary.BigEndian.PutUint64(corr[:], prev.CorrelationID)
					ids.ForgetContext(prev.DeliveryXID().Bytes(), corr)
				}

				return
			}
			// A removal runs before the purge report below, and interception stopping
			// is the outcome being reported — so it must actually have stopped.
			// Re-derivation happens on the enabler's worker, so asking is not
			// stopping: this waits for the pass it asked for, or the report would be a
			// lie in exactly the window it matters.
			enabler.retaskAndWait()
			// The numbering state goes with the tasking — **this task's context, not the
			// delivery XID's.**
			//
			// The granularity is the whole point here and it is not the same as at an
			// IRI-POI. The triggering function allocates one task per (warrant, session,
			// UPF) and gives them all the warrant's ProductID, which is what travels on
			// the wire as the XID, so several live tasks share it. Releasing by XID when
			// one PDU session ends would therefore restart the numbering of every other
			// session that warrant is still intercepting at this element — a sibling
			// session's next xCC numbered 0 where it should be 3.
			//
			// That would be worse than the leak releasing prevents. A sequence number is
			// how a mediation function detects loss, so numbering that resets under a
			// live context makes this element emit a sequence the receiver must read as
			// duplication or as a gap: the loss signal forged by the state that governs
			// it. Releasing one context leaves at worst an entry per ended session,
			// bounded by live tasking and visible through Identity.Contexts().
			//
			// Done on every removal — an ordinary withdrawal, a bulk deactivation, a
			// keepalive deactivation — because the state belongs to the tasking and not to the
			// circumstances of its removal.
			var corr [x2x3.CorrelationIDLength]byte
			binary.BigEndian.PutUint64(corr[:], prev.CorrelationID)
			ids.ForgetContext(prev.DeliveryXID().Bytes(), corr)
		}),
		// The keepalive procedure reports on the fault channel: fault 9050 when the
		// triggering function goes quiet, its clearing, and the 10000 alert once the
		// procedure has deactivated the tasking (TS 103 221-1 clause 6.6.2). An explicit
		// DeactivateTask, a retarget and a bulk deactivation are expected ends of an
		// interception and report nothing.
		x1.KeepaliveReporter(x1Reporter),
		// A task is refused unless every detection criterion is one this datapath can
		// resolve. Acknowledging one it cannot leaves the triggering function believing
		// an interception is running that can never produce anything — and nothing
		// outside this element could discover that.
		x1.CanApply(enabler.canApply),
		// Someone in the LI trust domain trying to trigger this CC-POI as a triggering
		// function it is not would be aiming a subject's traffic at a destination of
		// their choosing. It is refused, but this element logs nothing by design, so
		// without this the attempt leaves no trace at all.
		//
		// Reported off the X1 request goroutine. OnAuthFailure documents that it runs
		// synchronously on that goroutine and must not block, and a report is an mTLS
		// round trip to the ADMF bounded only by its own 10s timeout — so reporting a
		// refusal by holding the triggering interface open for the duration of a POST
		// to a peer that may itself be unreachable turns a refused request into a
		// stalled X1 channel, and makes this element's response time depend on whether
		// the ADMF is up. NotifyAsync keeps that contract in one place for the AMF,
		// the SMF and this element.
		x1.OnAuthFailure(func(code int) {
			if reporter != nil {
				reporter.NotifyAsync(x1.NEIssueX1AuthFailed,
					fmt.Sprintf("LI_T3 triggering refused: peer failed authentication (error %d)", code))
			}
		}),
	}
	// This element acts on a provisioned correlation value: it stamps task.CorrelationID
	// on every X3 PDU it delivers, and an LI_T3 trigger carries one mandatorily. So a
	// task carrying the field is accepted here, where an IRI-POI refuses it — the
	// correlation joining *its* records to a session is the session's, and one
	// provisioned value across the many sessions a task covers would collapse them at
	// the mediation function.
	opts = append(opts, x1.HonoursCorrelationID())

	// The two bulk operations the standard settles by advance agreement rather than by
	// what the element is. Unset leaves its defaults; li/x1 owns what unset means.
	opts = append(opts, x1.BulkOptions(cfg.DeactivateAllTasks, cfg.RemoveAllDestinations)...)

	srv := x1.NewServer(tasks, cfg.NEID, opts...)

	// ListenConfig.Listen rather than net.Listen so the listen carries a context
	// (the linter's noctx rule); the bind is otherwise unchanged.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", cfg.X1Listen)
	if err != nil {
		if reporter != nil {
			reporter.Notify(x1.NEIssueX1ListenFailed, "X1 listener bind failed")
		}

		return nil, fmt.Errorf("li: X1 listen on %s: %w", cfg.X1Listen, err)
	}

	// Mutual TLS with the identity binding checked per message by x1.Server: the
	// certificate proves the peer is in the LI domain, the binding proves it is the
	// triggering function we were told to accept.
	// NewListener supplies the properties every X1 endpoint needs and none of the
	// three network functions should be trusted to remember separately: a discarded
	// error log and per-phase timeouts, without which an unauthenticated peer can
	// hold connections open until this element can no longer be untasked.
	httpSrv := x1.NewListener(srv, serverTLS)
	// Certificates come from TLSConfig, so the file arguments are empty. ServeTLS
	// blocks until the listener is closed and then returns; the bind already
	// succeeded above, so its return is not actionable here.
	//nolint:errcheck // serve-until-close; a bind failure already surfaced above
	go func() { _ = httpSrv.ServeTLS(ln, "", "") }()

	// The keepalive procedure (TS 103 221-1 clause 6.6.2), applied to the triggering
	// interface for the reason it exists: a triggering function that goes away must not
	// leave interception running behind it. The same procedure guards the ADMF path on
	// the AMF and SMF. A nil stop channel means it runs for as long as this element can
	// hold tasking, which is the whole point.
	if keepalive != nil {
		go srv.WatchKeepalive(*keepalive, nil)
	}
	// Both edges of the task-scoped faults this element answers from state (clause
	// 6.5.2.1): a duplication the datapath is not applying, or a task selecting no
	// traffic, is reported when it appears and retracted once it has cleared.
	if x1Reporter != nil {
		go srv.WatchTaskFaults(x1Reporter, taskFaultInterval, taskFaultStop)
	}

	return tasks, nil
}

// taskFaultInterval is how often the task-fault watcher samples, and taskFaultStop ends
// it. Zero takes the X1 reporter's throttle interval and nil runs it for the life of the
// process; only tests set them, so a watcher started by one test does not outlive it.
var (
	taskFaultInterval time.Duration
	taskFaultStop     <-chan struct{}
)

// ccTFKeepaliveCadence is how often a CC triggering function tells this element it
// is still there. It is not configurable at the triggering function, so it is the
// fixed quantity this element's keepalive TIME_P2 has to be expressed against.
const ccTFKeepaliveCadence = 60 * time.Second

// minTriggerKeepalive is the shortest TIME_P2 that can distinguish a triggering
// function that has gone away from one that is merely between keepalives. Two
// cadences plus a margin: at one cadence any jitter, retransmit or scheduling delay
// reads as absence.
//
// Below it the keepalive procedure (TS 103 221-1 clause 6.6.2) stops being a backstop
// and becomes a fault: it reports fault 9050 against a healthy triggering function and,
// at TIME_P1 or TIME_P3, deactivates tasking that function is still answering for —
// sending an operator to investigate an element that was behaving correctly, while
// interception the agency believes is running has in fact stopped.
const minTriggerKeepalive = 2*ccTFKeepaliveCadence + 30*time.Second

// tooShortTriggerKeepalive reports whether a configured TIME_P2 is short enough to
// report and then deactivate tasking a healthy triggering function is still answering
// for.
//
// Separate from keepaliveProcedure, and checked only where operator configuration is
// validated, because the two answer different questions. keepaliveProcedure asks
// whether the timers are usable at all, and the shipper is built directly with short
// timers in tests that have to observe the procedure without waiting minutes for it.
// This asks whether a TIME_P2 an operator wrote is one the deployment can live with,
// which is a question only configuration can be wrong about.
func tooShortTriggerKeepalive(d time.Duration) bool {
	return d > 0 && d < minTriggerKeepalive
}

// keepaliveProcedure reads the keepalive procedure an operator configured, or nil when they
// disabled it (TS 103 221-1 clause 6.6.2: "By default (with no prior agreement) they are
// enabled", so only a stated agreement turns it off). A timer left empty takes the clause
// default.
//
// keepalive_timeout is TIME_P2; trigger_keepalive is its deprecated name and is still read.
// Both set to different values is refused rather than resolved, because the element cannot
// know which one the operator meant.
func keepaliveProcedure(cfg *LiConfig) (*x1.KeepaliveProcedure, error) {
	if cfg.KeepaliveDisabled {
		return nil, nil //nolint:nilnil // a disabled procedure is not an error
	}

	timeP2 := cfg.KeepaliveTimeout
	if cfg.TriggerKeepalive != "" {
		if timeP2 != "" && timeP2 != cfg.TriggerKeepalive {
			return nil, fmt.Errorf("li: keepalive_timeout %q and its deprecated name trigger_keepalive %q "+
				"are both set and differ; set keepalive_timeout only", timeP2, cfg.TriggerKeepalive)
		}
		timeP2 = cfg.TriggerKeepalive
	}

	// This element is triggered over LI_T3, where the triggering function plays the ADMF (TS
	// 33.128 clause 5.2.6). The reports go to the real ADMF, and its answer says nothing about
	// whether the triggering function is still there, so no answer starts TIME_P3: an orphaned
	// trigger is deactivated when TIME_P1 expires. keepalive_time_p3 would never run here and
	// is refused by Validate below.
	p := x1.KeepaliveProcedure{RetainTasking: cfg.KeepaliveRetainTasking, TriggeredOverLIT3: true}
	for _, t := range []struct {
		name, value string
		into        *time.Duration
	}{
		{"keepalive_timeout", timeP2, &p.TimeP2},
		{"keepalive_time_p1", cfg.KeepaliveTimeP1, &p.TimeP1},
		{"keepalive_time_p3", cfg.KeepaliveTimeP3, &p.TimeP3},
	} {
		d, err := parseKeepaliveTimer(t.name, t.value)
		if err != nil {
			return nil, err
		}
		*t.into = d
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &p, nil
}

// parseKeepaliveTimer reads one keepalive timer. Empty is the clause default, returned as
// zero; anything unparseable or non-positive is an error rather than the default, since a
// deployment that asked for a timer and silently got another holds tasking on terms nobody
// chose.
func parseKeepaliveTimer(name, v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("li: invalid %s %q", name, v)
	}

	return d, nil
}

// lookupTrigger returns the LI_T3 task covering the PFCP session identified by
// seid, and whether any does. The F-SEID is the
// detection criterion the CC-TF sends (TS 33.128 table 6.2.3-7, "PFCP Session
// ID") and the value the datapath tags onto every duplicated packet, so it is
// what ties a copy on the wire back to the warrant that authorised taking it.
//
// Where several warrants cover one session the first is chosen. That is a declared
// scope boundary rather than an unfinished piece of work, and it is declared in
// `li/CONFORMANCE.md` and its X1 disposition — what is not implemented, what it
// would cost, and what would make it worth revisiting are stated there and are
// deliberately not restated here, so the two cannot drift apart.
//
// What belongs here is why the code looks like this: "first" must be the same on
// every packet, and store.Match orders by XID precisely so this choice is stable.
// Picking from a map's iteration order instead — as this did — scattered one
// session's packets across the covering warrants at random, leaving every agency
// with a partial stream and none with a usable one. The filter returned is the chosen
// task's alone, so a copy it rejects is dropped, not offered to the next covering task:
// where the first task selects only part of a session — one tunnel direction, say —
// the rest of that session reaches no warrant. The warrants served nothing are told to
// the ADMF as a task fault against each of them (resolveTaskFaults), which is the part
// of this compromise that must not stay quiet. That fault walks the sessions the
// enabler has stored, so a session it does not yet hold — one whose rules reach the
// datapath before it is stored, which takes the fallback below — has its overlap
// reported only once it is stored; a shipper built with no enabler at all reports none.
func lookupTrigger(tasks *store.Store, enabler *ccEnabler, seid uint64) (types.InterceptTask, copyFilter, bool) {
	if tasks == nil || seid == 0 {
		return types.InterceptTask{}, copyFilter{}, false
	}

	// A task may be keyed by any of the detection criteria of table 6.2.3-7, most of
	// which name something other than a session — an address, a tunnel, a network
	// instance. Only the duplication control can say whether such a criterion selects
	// traffic in this session, since answering needs the session's own rules.
	matched := enabler.tasksCovering(seid)
	if len(matched) == 0 {
		// Without duplication control there is no session state to resolve against, so
		// the only criterion that can be answered is the session identity itself, which
		// covers a whole session and so needs no filtering. This is the path a shipper
		// built without a datapath takes.
		session := types.TargetIdentifier{Type: types.TargetFSEID, Value: strconv.FormatUint(seid, 10)}
		for _, task := range tasks.Match(session) {
			// Every criterion must match, and without session state only the session
			// identity can be answered: a task naming it beside anything else cannot be
			// decided here, and attributing the copy to it would deliver traffic its other
			// criteria may exclude.
			if !task.MatchedBy([]types.TargetIdentifier{session}) {
				continue
			}
			// The same filter tasksCovering applies, and it matters here for the reason
			// stated there: the caller takes the first task, so a task that requires no
			// content — and therefore has no X3 destination to deliver it to — would
			// take attribution of the copy away from the warrant that does, and swallow
			// the whole stream. Applying it on one path and not the other means the
			// answer depends on whether a datapath happened to be attached.
			if !producesCC(task) {
				continue
			}
			matched = append(matched, coveredTask{task: task, filter: unfiltered()})
		}
	}
	if len(matched) == 0 {
		return types.InterceptTask{}, copyFilter{}, false
	}

	return matched[0].task, matched[0].filter, true
}
