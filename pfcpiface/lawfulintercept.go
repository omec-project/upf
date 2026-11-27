// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// The BESS liEncap (GenericEncap) prepends a fixed tag to every duplicated
// packet before it reaches the X3 egress socket: an 8-byte F-SEID (used as the
// X3 correlation id) followed by a 1-byte FAR action (the forward-and-duplicate
// action, which encodes the direction and whether the copy is GTP-U encapsulated).
// Layout: [fseid(8)][action(1)][packet].
const (
	fseidTagLen = 8
	liTagLen    = fseidTagLen + 1 // + 1-byte FAR action
)

// reconnect backoff bounds for the X3 egress socket.
const (
	minReconnectDelay = 100 * time.Millisecond
	maxReconnectDelay = 5 * time.Second
)

// Punting copies to userspace has a ceiling, and the socket that carries them
// holds only as many datagrams as net.unix.max_dgram_qlen allows — 10 by default,
// which a burst fills instantly. Everything the read loop does before returning to
// Read is time that queue spends filling, so the loop does nothing but read and
// hand off; parsing, task lookup and PDU framing happen on workers.
const (
	// liFrameQueueDepth absorbs a burst that arrives faster than it can be framed. It is
	// the total across the workers' queues, so the buffers it can hold do not grow with
	// the number of workers.
	liFrameQueueDepth = 4096
	// liFrameWorkers frame concurrently. Delivery itself is already asynchronous;
	// this covers the per-packet work in front of it. Each session is framed by one of
	// them, so its copies are framed in the order they were read: see frameQueueFor.
	liFrameWorkers = 4
	// liMaxPunted bounds a single copy read from the socket.
	liMaxPunted = 1 << 16
)

// liShipper is the UPF's Lawful Interception CC-POI. It reads the content-of-
// communication copies the BESS datapath tees to a userspace socket (for a FAR
// carrying the DUPL apply-action) and delivers each one to the MDF3 as an ETSI
// TS 103 221-2 X3 PDU over mutual TLS. Opt-in: created only when LI is
// configured, and it logs nothing that reveals which subscriber is intercepted.
type liShipper struct {
	sockAddr string
	// rcvBuf is the punt-socket receive buffer this element was configured with, kept
	// so reconnect can re-apply it. A reconnected socket is a new socket and starts
	// at the kernel default: the deepening survives the first dial and not the
	// second, so an element that has reconnected once quietly runs with a fraction
	// of the egress capacity it was deployed with, and the only symptom is copies
	// dropped under burst.
	rcvBuf   int
	sock     net.Conn
	reporter neIssueReporter // nil when NE-initiated reporting is not configured
	// tasks holds the LI_T3 triggers installed by the CC-TF, indexed by the
	// F-SEID the datapath tags onto duplicated packets. It is what supplies the
	// warrant XID and correlation identifier for each copy.
	tasks *store.Store
	// enabler resolves a task's detection criteria against PFCP session state. The
	// shipper needs it because a task's criterion need not be the session identity
	// the datapath tags copies with. Nil only in a test that built a shipper
	// directly, where the F-SEID criterion is all that can be answered.
	enabler *ccEnabler
	// tlsConfig is the client side of the LI PKI, used to dial each MDF3.
	tlsConfig *tls.Config
	// keepalive is the TS 103 221-2 clause 6.2.4 mechanism as configured, applied to
	// every X3 connection this shipper opens. This POI keeps its own clients rather
	// than using x2x3.Pool, so the settings are carried here to the one place that
	// builds them — the alternative being an X3 interface that quietly runs a
	// different mechanism from the two X2 ones.
	keepalive x2x3.KeepaliveConfig
	// ids is what every PDU this element sends carries besides the task's own
	// labels: the two constant element identities and the sequence numbering. Shared
	// with the IRI-POIs through li/x2x3, so the three points of interception cannot
	// come to disagree about how an attribute is built.
	ids *x2x3.Identity

	// watcher reports both edges of a destination's reachability — that it failed
	// and that it recovered. Nil when NE-initiated reporting is not configured, and
	// its methods are nil-safe, so the delivery path calls it unconditionally.
	watcher *x1.DestinationWatcher

	mu      sync.Mutex
	senders map[string]x2x3.Sender // per MDF3 address, created on first use

	// egressDown records that the datapath egress socket is not connected — from the
	// moment a read on it fails until a redial succeeds. It is the state behind the
	// x3EgressDown probe, and it is a separate flag rather than a look at sock because
	// sock is the shipping loop's to write and a probe answers on another goroutine.
	egressDown atomic.Bool

	// punted carries copies from the socket read to the framing workers, one queue per
	// worker, and free recycles the buffers so the hot path does not allocate per packet.
	punted []chan []byte
	free   chan []byte
}

// errNoDeliveryCredentials means X3 delivery was attempted with no LI client
// credentials loaded, which can only happen in a test that built a shipper
// directly. Delivering intercept product over an unauthenticated connection is
// never an acceptable fallback.
var errNoDeliveryCredentials = errors.New("li: no X3 delivery credentials")

// errNoElementIdentifier means the deployment configured interception without the
// identifier this element asserts on X1, which is also the Network Function ID every
// xCC it delivers has to carry (TS 33.128 table 5.3.1-2).
var errNoElementIdentifier = errors.New("li: no network element identifier configured")

// upfInterceptionPoint is the Interception Point ID every xCC from this element
// carries (ETSI TS 103 221-2 clause 5.3.8): it names the POI *within* the network
// function, and this network function contains exactly one — the CC-POI that ships
// duplicated content. Format is left to the implementation, so it is a name rather
// than a number, because whoever reads it is correlating with an NFID that is also a
// name.
const upfInterceptionPoint = "UPF-CC-POI"

// neIssueReporter surfaces LI-plane faults to the ADMF over X1. An interface (like
// the x2x3.Sender above) so tests can assert what a fault reports without an ADMF.
// It exposes the fire-and-forget forms (*x1.Reporter.Notify and NotifyAsync):
// reporting is best-effort by design and a failed report has nowhere to go, so the
// outcome is not returned.
//
// Two forms because two kinds of caller. NotifyAsync consults the throttle on the
// calling goroutine and dispatches the POST off it, which is what every report raised
// from the shipping loop, a framing worker or the X1 request goroutine must use: a
// report is an mTLS round trip bounded only by its own 10s timeout, and those paths
// may not wait for one. Notify is the blocking form, and what still uses it is
// startup — a configuration this element will not run with, reported before there is
// a loop to stall.
type neIssueReporter interface {
	Notify(issueType, description string)
	NotifyAsync(issueType, description string)
}

// startLIShipper dials the datapath's X3 egress socket, prepares X3 delivery to
// the MDF3 (mutual TLS), and starts the shipping loop.
func startLIShipper(cfg *LiConfig, client pb.BESSControlClient, u *upf) error {
	// Without an identifier for this network element, content would reach a mediation
	// function that cannot attribute it to the element that produced it. Interception
	// does not start; the datapath carries on forwarding, because a UPF that refuses to
	// come up over its LI configuration tells every operator it is LI-provisioned.
	if cfg.NEID == "" {
		return errNoElementIdentifier
	}

	mat, err := mtls.Load(cfg.Cert, cfg.Key, cfg.CACert)
	if err != nil {
		return err
	}

	// The reporter is built before anything that can fail needs it, and in particular
	// before the datapath socket is dialled: that is the step most likely to fail —
	// the one thing here that depends on another container having got there first —
	// and it needs somewhere to report its failure.
	var reporter *x1.Reporter
	if cfg.AdmfURL != "" {
		reporter = x1.NewReporter(cfg.AdmfURL, cfg.AdmfID, cfg.NEID, mat.ClientTLS())
	}

	// Bring up the LI_T3 triggering interface before the shipping loop: without it
	// no content can be attributed to a warrant, so there is nothing worth shipping.
	// A typed-nil reporter would defeat the nil checks inside, so pass the
	// interface only when one is configured.
	var issueReporter neIssueReporter
	if reporter != nil {
		issueReporter = reporter
	}

	// The configuration is checked here, and not in validateConf, because a refusal
	// there would take the user plane down and name the offending LI field in the general
	// operator log. Here it stops interception, tells the ADMF, and leaves the datapath
	// forwarding — the same policy the AMF and SMF apply to their own LI block.
	//
	// The refused `li` object, first, because it is the reason the rest cannot be trusted: the
	// keys that did decode are not a trustworthy set, and one of the ones that did not may be
	// a keepalive timer or the fault endpoint.
	if cfg.blockErr != nil {
		if issueReporter != nil {
			issueReporter.Notify(x1.NEIssueInvalidConfig,
				"this element's interception configuration carries a setting it does not "+
					"recognise or cannot read, so the values it would fall back on cannot be trusted; "+
					"interception has not been started")
		}

		return cfg.blockErr
	}

	if cfgErr := validateLiConfig(cfg); cfgErr != nil {
		if issueReporter != nil {
			issueReporter.Notify(x1.NEIssueInvalidConfig,
				"the content-interception configuration is unusable, so interception has not been started")
		}

		return cfgErr
	}

	// Duplication control before the listener, because the listener refuses tasking
	// it cannot carry out and needs this to answer. u is nil only in a test that
	// builds a shipper without a datapath.
	var enabler *ccEnabler
	if u != nil {
		enabler = u.newInterceptionEnabler(func(issueType, description string) {
			if issueReporter != nil {
				issueReporter.NotifyAsync(issueType, description)
			}
		})
		u.ccEnabler = enabler
	}

	// X3 destinations arrive per task over LI_T3 (CreateDestination + the task's
	// ListOfDIDs), so no delivery client can be built here — one is created per
	// MDF3 address on first use. Delivery is asynchronous either way: shipLoop must
	// keep draining the datapath socket and cannot block on an MDF3, or the unread
	// socket would make BESS drop every subsequent copy.
	//
	// Built before the triggering interface because that interface answers for it: the two
	// conditions this element can be asked about — whether its mediation functions are
	// reachable, and whether the datapath egress is up — are the shipper's to know, and
	// nothing else in this process can see either.
	s := &liShipper{
		sockAddr:  cfg.X3SockAddr,
		rcvBuf:    cfg.X3RcvBuf,
		enabler:   enabler,
		tlsConfig: mat.ClientTLS(),
		keepalive: keepaliveConfig(*cfg, issueReporter),
		senders:   make(map[string]x2x3.Sender),
		punted:    make([]chan []byte, liFrameWorkers),
		free:      make(chan []byte, liFrameQueueDepth),
		// Built once: the two identities are constant for the life of the process, so
		// no xCC pays for constructing them, and this is a per-packet path.
		ids: x2x3.NewIdentity(cfg.NEID, upfInterceptionPoint),
	}
	for i := range s.punted {
		s.punted[i] = make(chan []byte, liFrameQueueDepth/liFrameWorkers)
	}
	// The egress is down until the shipping loop has established it, which is the
	// truth from this moment and the answer the x3EgressDown probe owes anyone who
	// asks in between. sock stays nil until then; nothing but shipLoop reads it.
	s.egressDown.Store(true)
	// Only assign when configured: a typed-nil *x1.Reporter in the interface field
	// would pass the nil check in report() and then panic on use.
	if reporter != nil {
		s.reporter = reporter
	}

	// Before the egress, deliberately. A CC-POI whose datapath has not arrived can
	// still be tasked and can still be asked how it is — which is a reportable state
	// a triggering function can act on. One that abandoned its own initialisation is
	// in no state at all, and is indistinguishable from an element nobody tasked.
	tasks, err := startTriggerListener(cfg, mat.ServerTLS(), issueReporter, enabler, s.ids,
		s.reclaimUnreferencedSenders, s.addressUnreachable, s.faultProbes()...)
	if err != nil {
		// The enabler was started above and owns a worker goroutine. A partial
		// initialisation that returns an error has to leave nothing running, or a
		// process that retries the bind — or simply reports the failure and carries on
		// serving, which is what this element does rather than crash-loop — accumulates
		// a worker per attempt, each holding the session stores it was given.
		enabler.stop()
		// And clear it, or every subsequent session establishment runs applyTasking
		// against a stopped enabler and asks for passes nothing will ever serve. Such a
		// request is a counter bump with no worker behind it, but the field is what tells
		// the session path there is an interception plane to consult: leaving it set says
		// yes when the answer is no.
		if u != nil {
			u.ccEnabler = nil
		}

		return err
	}
	s.tasks = tasks

	// One owner for both edges of a destination's reachability: that it failed, and
	// that it recovered. If the delivery and keepalive paths reported the first,
	// nothing would report the second, and an ADMF told a destination was unreachable
	// would never be told it came back. A nil stop channel runs for as long as this
	// element can hold tasking, which is the right lifetime.
	//
	// Before the delivery workers start, so a fault seen by the first copy shipped
	// has somewhere to go.
	if reporter != nil {
		s.watcher = x1.NewDestinationWatcher(s.destinationHealth, reporter, 0)
		go s.watcher.Watch(nil)
	}

	for _, q := range s.punted {
		go s.frameLoop(q)
	}

	// shipLoop establishes the egress itself, retrying until the datapath answers,
	// rather than dialling it here once: a single failed dial would abandon
	// interception for the life of the process. The two containers start
	// concurrently with no readiness gate between them, so "the socket is not there
	// yet" is the ordinary case rather than the exotic one, and every other peer this
	// element depends on is already retried without limit.
	go s.shipLoop()

	// Loss between the datapath and this shipper is invisible from here — a copy
	// discarded on the socket write never arrives — so it is watched from the only
	// vantage point that can see it, the datapath's own accounting.
	startLIPuntMonitor(client, issueReporter)

	return nil
}

// newInterceptionEnabler builds the duplication control for this UPF's datapath.
//
// The datapath's cause is carried back rather than discarded. A FAR it refused is not
// recorded as programmed, so the next re-derivation attempts it again — and the refusal is
// reported, since an interception this element accepted and is not carrying out is a
// condition only the ADMF can act on. These writes are made outside any PFCP procedure, so
// they go through the datapath's untimed path (sendDuplicationPush).
func (u *upf) newInterceptionEnabler(report func(issueType, description string)) *ccEnabler {
	return newCCEnabler(nil, u.sendDuplicationPush, report)
}

// setPuntReadBuffer asks the kernel for a deeper receive buffer on the egress
// socket, so a burst of duplicated packets is absorbed rather than discarded on
// the datapath's write.
//
// On its own this achieves little, and it is worth being explicit about why: an
// AF_UNIX SEQPACKET socket refuses a write once its receive queue holds
// net.unix.max_dgram_qlen *datagrams*, which defaults to 10 and is not affected by
// the buffer size at all. The byte budget only starts to bind after that queue
// limit is raised, which is a deployment matter — the sysctl is per network
// namespace, so the pod needs it set before this socket is created.
func setPuntReadBuffer(sock net.Conn, size int) {
	if size <= 0 {
		return
	}

	if c, ok := sock.(*net.UnixConn); ok {
		// Best-effort: the kernel caps the request at net.core.rmem_max, and a
		// smaller-than-requested buffer is still better than the default. Nothing is
		// logged either way — a failure here degrades capacity, and capacity
		// problems are reported by the egress monitor rather than announced.
		//nolint:errcheck // best-effort tuning; degraded capacity is reported by the egress monitor
		_ = c.SetReadBuffer(size)
	}
}

// faultProbes are the conditions this element can answer for when its triggering function
// asks how it is. Both are states — re-observable at the moment of asking — which is what
// keeps them here and keeps the lost-copy counters in the pushed reporting instead.
//
// They are two probes rather than one because they are two faults with two responses: an
// unreachable mediation function is a problem outside this element, a dead egress socket is
// one inside it, and either can hold while the other does not. Sharing a probe would let
// whichever was evaluated first stand for both.
func (s *liShipper) faultProbes() []x1.FaultProbe {
	return []x1.FaultProbe{
		x1.MDFUnreachableProbe(s.unreachableDestinations),
		func() *x1.X1Error {
			if !s.egressDown.Load() {
				return nil
			}

			// The socket's present state, not a history of what was lost while it was down.
			// Copies dropped at the egress are events with a pushed report of their own
			// (x3PuntLost), and accumulating them here would leave this element permanently
			// faulty long after the datapath came back.
			return x1.NEFault(x1.NEIssueX3EgressDown,
				"the datapath content egress socket is not connected")
		},
	}
}

// unreachableDestinations counts the MDF3s this element's triggers currently name that it
// cannot reach, and how many of them it has attempted at all.
//
// It is x2x3.Pool.UnreachableAmong's answer for a shipper that keeps its own clients — this
// element must refuse delivery outright when it has no credentials loaded, which the pool
// does not do. A destination nothing has been sent to is not counted: an element with nothing
// to deliver has not found an MDF3 unreachable, it has not looked.
//
// Only destinations still under trigger, and here that matters more than anywhere else: a
// UPF's triggers are withdrawn as sessions end, several times an hour. A destination whose last
// delivery failed and whose trigger was then removed can never be delivered to again, so if it
// were counted nothing would ever clear it and this element would report itself faulty for the
// life of the process — while holding no tasking at all.
func (s *liShipper) unreachableDestinations() (unreachable, inUse int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	counted := make(map[string]bool)
	for _, addr := range s.destinationsInUse() {
		if counted[addr] {
			continue
		}
		counted[addr] = true

		sender, ok := s.senders[addr]
		if !ok {
			continue
		}
		inUse++
		if r, ok := sender.(x2x3.Reachability); ok && r.Unreachable() {
			unreachable++
		}
	}

	return unreachable, inUse
}

// destinationsInUse is every X3 endpoint of every trigger this element holds — all
// of them, since delivery fans out to all of them and an unreachable second
// destination is as much a fault as an unreachable first. Nil tasks — a shipper
// built directly in a test — name nothing.
func (s *liShipper) destinationsInUse() []string {
	if s.tasks == nil {
		return nil
	}

	var addrs []string
	for _, task := range s.tasks.Snapshot() {
		addrs = append(addrs, task.DeliveryAddresses(types.DeliveryX3)...)
	}

	return addrs
}

// reclaimUnreferencedSenders closes and drops the delivery client for every address no
// trigger this element holds still names.
//
// **Without this `senders` is append-only.** A destination is created on first use, so
// destination churn — a warrant relabelled to a new MDF3, an agency's endpoint moved, a
// triggering function that re-provisions after its own restart — would accumulate a delivery
// worker, a TLS connection and a keepalive timer per address for the life of the process. On
// an element that also keepalives each of them, that is a growing set of connections to
// mediation functions this element no longer answers for, and the keepalives on them are
// indistinguishable at the far end from an element that is still delivering.
//
// Called wherever the tasking changes, which is the same event that changes the answer to
// "which destinations are in use". Closing is what stops the worker and the keepalive; the
// sender drains what it holds first, bounded by its own deadline.
func (s *liShipper) reclaimUnreferencedSenders() {
	s.mu.Lock()

	inUse := make(map[string]bool)
	for _, addr := range s.destinationsInUse() {
		inUse[addr] = true
	}

	var stale []x2x3.Sender
	for addr, sender := range s.senders {
		if inUse[addr] {
			continue
		}
		stale = append(stale, sender)
		delete(s.senders, addr)
	}
	s.mu.Unlock()

	// Outside the lock: Close drains the queue, and the delivery path takes this lock.
	for _, sender := range stale {
		//nolint:errcheck // a close failure loses nothing that was not already going
		_ = sender.Close()
	}
}

// close tears the shipper down: every delivery client it holds, whether or not a trigger
// still names it.
//
// Without it the shipper's senders would live as long as the process, so a reconfiguration or
// a shutdown would leave workers running and connections open to mediation functions with
// nothing left to send them.
func (s *liShipper) close() {
	s.mu.Lock()
	senders := s.senders
	s.senders = make(map[string]x2x3.Sender)
	s.mu.Unlock()

	for _, sender := range senders {
		//nolint:errcheck // teardown; see reclaimUnreferencedSenders
		_ = sender.Close()
	}
}

// destinationHealth is the watcher's view of the same destinations
// unreachableDestinations counts: the identifier the ADMF provisioned each under,
// and whether this element can currently reach it.
//
// It is a different shape from the probe's on purpose. The probe answers a status
// request and takes counts so it *cannot* name a destination — an element's own
// status says how much is wrong, never whose product is affected. A
// destination-scoped report says which, because the ADMF asked about none in
// particular and that is exactly what it needs. Same fact, two questions.
func (s *liShipper) destinationHealth() []x1.DestinationHealth {
	if s.tasks == nil {
		return nil
	}

	// The UPF has no configured fallback for X3: a task carrying no X3 destination is
	// refused at ship time rather than delivered somewhere this element chose. So what
	// it delivers to is what the task names, and the resolver says exactly that.
	return x1.DestinationHealthOf(s.tasks.Snapshot(), types.DeliveryX3,
		func(t types.InterceptTask) []string { return t.DeliveryAddresses(types.DeliveryX3) },
		s.addressUnreachable)
}

// addressUnreachable answers for one endpoint what unreachableDestinations counts
// over all of them. It performs no I/O, which is what makes it safe to sample on a
// timer.
func (s *liShipper) addressUnreachable(addr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sender, ok := s.senders[addr]
	if !ok {
		// Nothing has been sent to it, so this element has not found it unreachable —
		// it has not looked. The same rule unreachableDestinations applies.
		return false
	}
	r, ok := sender.(x2x3.Reachability)

	return ok && r.Unreachable()
}

// report surfaces an LI-plane fault to the ADMF over X1 (throttled, NE-level, no
// target id), never to general logs. No-op when reporting is not configured.
//
// **Every caller of this is a path that may not block, so it dispatches off the
// caller's goroutine.** The read loop reports the egress being down and copies dropped
// before framing; the framing workers report untasked content, an unusable
// configuration, an unreachable MDF and an invalid tag; the delivery worker
// reports copies dropped from its queue. All of them are driven by packet rate, in
// front of a datagram queue whose whole invariant is that everything done before
// returning to Read is time that queue spends filling — so a synchronous mTLS POST
// here would make the report that says copies were dropped the reason the next
// several hundred are. NotifyAsync takes the throttle decision here (a mutex, no I/O) and
// spawns only for a report that is actually going out, which is why this is safe at
// packet rate and plain `go` would not be.
func (s *liShipper) report(issueType, description string) {
	if s.reporter != nil {
		s.reporter.NotifyAsync(issueType, description)
	}
}

// shipLoop reads each teed packet — an 8-byte F-SEID tag (prepended by the BESS
// GenericEncap) followed by the subscriber's inner IP packet — and hands it to
// ship, which resolves the F-SEID to the LI_T3 task covering that session and
// delivers the content labelled with that task's warrant XID and correlation
// identifier.
func (s *liShipper) shipLoop() {
	// The datapath socket is established here rather than at construction, so that a
	// datapath which is not up yet delays interception instead of cancelling it. This
	// blocks until it answers; everything else this element does — being tasked,
	// answering for its own state — is already running by now.
	s.establish()

	buf := make([]byte, liMaxPunted)

	for {
		n, err := s.sock.Read(buf)
		if err != nil {
			// The datapath X3 socket dropped (BESS restart / transient error). Do
			// not die — that would silently disable interception for the life of
			// the process. Report it to the ADMF, then reconnect (with backoff) so
			// interception resumes when the datapath returns.
			s.report(x1.NEIssueX3EgressDown, "X3 egress socket unavailable")
			s.reconnect()

			continue
		}

		if n <= liTagLen {
			continue // tag only, no user-plane payload
		}

		// Copy out of the read buffer and hand off. The read buffer is reused, so
		// the copy is necessary; doing anything more here — parsing, a task lookup,
		// framing a PDU — is time the socket queue spends filling behind us, and
		// what it overflows with is intercept product nobody can recover.
		out := append(s.buffer()[:0], buf[:n]...)

		select {
		case s.frameQueueFor(out) <- out:
		default:
			// Framing cannot keep up. This is lost content like any other, so it is
			// reported rather than left to be inferred from a counter nobody reads.
			s.recycle(out)
			s.report(x1.NEIssueX3FramingLost, "content copies dropped before framing")
		}
	}
}

// frameQueueFor picks the framing worker for a copy by the session in its tag.
//
// **A session's copies are framed by one worker, in the order they were read.** A
// fragmented datagram is classified from its initial fragment and its later fragments
// follow that decision (lawfulintercept_fragment.go). With any worker free to take any
// copy, a later fragment read after its first could be looked up before the worker
// holding the first had classified it, and be dropped and reported as lost although it
// arrived in order. The memo is per session, so ordering within a session is all that
// decision needs. The tag is already in hand, so the choice costs a modulo and no
// parsing on the read loop. The local SEID is drawn at random, so sessions spread over
// the workers; a single session is framed by one.
func (s *liShipper) frameQueueFor(tagged []byte) chan []byte {
	fseid := binary.LittleEndian.Uint64(tagged[:fseidTagLen])

	return s.punted[fseid%uint64(len(s.punted))]
}

// frameLoop turns punted copies into X3 PDUs and hands them to delivery.
func (s *liShipper) frameLoop(punted <-chan []byte) {
	for b := range punted {
		if s.checkTag(b) {
			s.ship(b)
		}
		s.recycle(b)
	}
}

// buffer takes a recycled buffer, or allocates when the pool is empty.
func (s *liShipper) buffer() []byte {
	select {
	case b := <-s.free:
		return b
	default:
		return make([]byte, 0, liMaxPunted)
	}
}

// recycle returns a buffer for reuse, dropping it if the pool is full.
func (s *liShipper) recycle(b []byte) {
	select {
	case s.free <- b:
	default:
	}
}

// ship delivers one duplicated packet, if the session it came from is covered by
// an LI_T3 task.
//
// Duplication and tasking reach this UPF over different interfaces — the DUPL
// apply-action over PFCP, the warrant over X1 — so they can disagree.
// Content whose session has no task is **dropped and reported**, never delivered:
// the only label available for it would be a zero XID, and a mediation function
// attributes product by XID alone and discards what it cannot attribute without
// complaint. Shipping it would look like working interception while delivering
// nothing usable.
func (s *liShipper) ship(tagged []byte) {
	fseid := binary.LittleEndian.Uint64(tagged[:fseidTagLen])

	task, filter, ok := lookupTrigger(s.tasks, s.enabler, fseid)
	if !ok {
		s.report(x1.NEIssueContentUntasked, "duplicated content for a session with no interception task")

		return
	}

	// Duplication is set per FAR, so a copy may arrive that the task's detection
	// criteria do not identify — another PDR's traffic through the same FAR, or
	// another transport port. Dropped silently and deliberately not reported: a copy
	// that did not match is not lost content but content this element was never
	// authorised to take, and reporting it as a fault would manufacture delivery
	// failures out of correct behaviour.
	if !filter.matches(tagged[fseidTagLen], tagged[liTagLen:]) {
		return
	}

	// Every destination the task named for this product, and only those: a destination
	// provisioned as X2Only is not one, since delivering content to a signalling
	// endpoint would be a disclosure to the wrong place. DeliveryAddresses filters on
	// the delivery type and deduplicates, so two DIDs naming one endpoint deliver once.
	dests := task.DeliveryAddresses(types.DeliveryX3)
	if len(dests) == 0 {
		s.report(x1.NEIssueInvalidConfig, "interception task carries no X3 delivery destination")

		return
	}

	// Framed once, before the fan-out, and shared by every destination. Two things
	// must not happen per destination. The sequence number is taken in here, and it
	// belongs to the (XID, Correlation Identifier) context rather than to a
	// connection (TS 103 221-2 clause 5.3.9) — numbering per destination would give
	// each mediation function a stream whose gaps are indistinguishable from loss.
	// And the framing is the per-packet cost this path is measured on, so a second
	// destination must cost its own delivery and nothing else.
	//
	// Sharing the value is safe under AsyncSender.Send's contract: the caller must
	// not mutate a PDU after Send, and nothing here does. Each client marshals it
	// into its own write.
	pdu := shipperPDU(tagged, task, s.ids)

	for _, dest := range dests {
		sender, err := s.senderFor(dest)
		if err != nil {
			// One mediation function this element cannot deliver to does not deny the
			// others the warrant's product. The address is named because the triggering
			// function provisioned several and otherwise cannot tell which one failed.
			s.report(x1.NEIssueMDFUnreachable,
				"X3 delivery destination could not be prepared: "+dest)

			continue
		}

		// Enqueue for asynchronous delivery and keep reading: Send never blocks, so
		// a slow/unreachable MDF3 cannot stall the socket read (which would make
		// BESS drop subsequent copies). Delivery failures are reported from the
		// worker via the onError hook set in senderFor.
		//nolint:errcheck // async enqueue never blocks; delivery failures report via onError
		_ = sender.Send(pdu)
	}
}

// keepaliveConfig turns the operator's three settings into the clause 6.2.4
// mechanism's configuration.
//
// It encodes no defaults of its own: an unset timer is passed through as zero, which
// x2x3 resolves to the specification's own value — so there is one place where "60
// seconds" is written down, beside the mechanism rather than in each of the three
// network functions that run it.
//
// An unusable value is **reported and then discarded in favour of those defaults**,
// which is what the AMF and SMF do. Refusing would be the wrong instrument twice: the
// specification supplies these two defaults normatively, so there is something correct
// to fall back to, and a refusal would reach Fatalln — stopping the user plane over a
// timer, and naming the LI field in the general operator log while doing it.
func keepaliveConfig(cfg LiConfig, reporter neIssueReporter) x2x3.KeepaliveConfig {
	ka := x2x3.KeepaliveConfig{
		Disabled: cfg.X2X3KeepaliveEnabled != nil && !*cfg.X2X3KeepaliveEnabled,
	}

	report := func(description string) {
		if reporter != nil {
			reporter.Notify(x1.NEIssueInvalidConfig, description)
		}
	}

	for name, t := range map[string]struct {
		value string
		into  *time.Duration
	}{
		"x2x3_keepalive_time_p1": {cfg.X2X3KeepaliveTimeP1, &ka.TimeP1},
		"x2x3_keepalive_time_p2": {cfg.X2X3KeepaliveTimeP2, &ka.TimeP2},
	} {
		d, err := parseOptionalDuration(t.value)
		if err != nil {
			report(name + " is not a duration; the specification's default is used instead")

			continue
		}
		*t.into = d
	}

	if err := ka.Validate(); err != nil {
		report("the configured X2/X3 keepalive timers are unusable and the specification's " +
			"defaults are used instead: " + err.Error())

		return x2x3.KeepaliveConfig{Disabled: ka.Disabled}
	}

	return ka
}

// parseOptionalDuration parses a Go duration, treating empty as unset (zero).
func parseOptionalDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	return time.ParseDuration(s)
}

// senderFor returns the delivery client for an MDF3 address, creating it on first
// use. Destinations arrive per task over X1, so they are not known at startup, and
// several agencies' destinations may be in use at once — hence one sender per
// address rather than one for the process.
func (s *liShipper) senderFor(addr string) (x2x3.Sender, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sender, ok := s.senders[addr]; ok {
		return sender, nil
	}

	if s.tlsConfig == nil {
		return nil, errNoDeliveryCredentials
	}

	// Neither the keepalive nor a delivery failure reports here, although the
	// condition matters.
	//
	// Both establish that this destination is unreachable, which is a condition the
	// element can *re-observe* — the sender knows it, which is what
	// unreachableDestinations and the watcher both ask. So it has an ending, and an
	// ending has to be reported (TS 103 221-1 clause 5.3). A site that announces and
	// nothing that retracts eventually announces something nobody retracts, and both
	// halves belong to whoever can see the transition. That is the watcher, which
	// reports the failure naming the destination it concerns and the recovery when it
	// comes.
	//
	// The callbacks are wired to nudge the watcher instead of reporting. The
	// sender marks itself unreachable independently of them — sendBytes and the
	// keepalive expiry both store it before the callback runs — so what the nudge
	// buys is not the observation but its promptness: without it the ADMF would learn
	// of a failed destination one sampling interval after this element did.
	keepalive := s.keepalive
	keepalive.OnFault = s.reportDeliveryError

	sender := x2x3.NewAsyncSender(
		x2x3.NewClient(addr, s.tlsConfig, keepalive), 0,
		s.reportDeliveryError,
		// A full queue is lost content, and it is not covered by the delivery-failure
		// report above: that fires when the MDF is unreachable, whereas the queue
		// overflows when the MDF is reachable but slower than the offered rate. Left
		// unreported, the product is silently incomplete.
		func() { s.report(x1.NEIssueX3DeliveryLost, "content copies dropped from the delivery queue") },
	)
	s.senders[addr] = sender

	return sender, nil
}

// reportDeliveryError is what this element does with a failure its delivery worker or its
// keepalive reports — both hooks, because both carry the same errors.
//
// **The error is inspected, not discarded.** ErrUnitDropped says delivery to this MDF3 is
// working and one content copy of it was lost — a unit the write stopped inside, which the
// library resends whole on a fresh connection and reports as dropped only where that resend
// did not land either. The library deliberately refuses to call that unreachability, because
// a healthy mediation function must not be reported unreachable over one truncated write; if
// these hooks discarded the error, the loss would be reported by nothing while the watcher goes
// on sampling a destination it correctly considers reachable. Content missing from an agency's
// record with every channel that could have said so reporting normality.
//
// Reported as x3DeliveryLost, the same condition a full delivery queue raises: from the
// agency's side the two are one fact — a copy this element made and did not deliver.
//
// A method rather than a closure inside senderFor so the mapping can be asserted directly.
// What produces ErrUnitDropped is the transport, tested where it lives in li/x2x3; what this
// element does with it is this function.
//
// The report says how many copies the drop cost. A write that stops inside one unit is resent
// from that unit to the end of its batch, so a resend that does not land loses every copy in
// that tail, and the library counts them (x2x3.LostUnits). Saying "a content copy" for what may
// be a run of them would leave the agency's gap larger than this element's own account of it.
func (s *liShipper) reportDeliveryError(err error) {
	if errors.Is(err, x2x3.ErrUnitDropped) {
		s.report(x1.NEIssueX3DeliveryLost, droppedCopies(x2x3.LostUnits(err)))
	}
	s.watcher.Nudge()
}

// droppedCopies describes a drop of n content copies on the way to a reachable mediation
// function. An error that carries no count (n is 0) still lost at least the copy it names, so it
// is not described as a number this element cannot back.
func droppedCopies(n int) string {
	switch {
	case n > 1:
		return fmt.Sprintf("%d content copies were partially written to a reachable mediation "+
			"function and dropped", n)
	case n == 1:
		return "a content copy was partially written to a reachable mediation function and dropped"
	default:
		return "at least one content copy was partially written to a reachable mediation function " +
			"and dropped"
	}
}

// checkTag reports whether a punted copy's datapath tag can be trusted enough to
// ship. A copy that cannot is dropped rather than delivered.
//
// A tag can be wrong at all because its fields reach liEncap as BESS per-packet
// metadata, whose offsets are assigned from the pipeline graph: a wiring in which they
// do not reach the encap is accepted at load time with only a printed note, and every
// copy then carries a zero correlation id or an unknown action. Either way the fault is
// reported over X1 and never to general logs.
//
// The action byte selects the X3 payload format and direction, so an unknown value
// leaves no correct label to apply: shipping it anyway sends the mediation function
// content asserting a direction the element does not actually know, which is worse
// than the copy going missing. A missing copy is a gap in a sequence the MDF can
// see; a mislabelled one is a fact about a subject that is untrue and carries no
// sign of it.
//
// A zero correlation is reported but not fatal to the copy: the XID still
// attributes it to the right warrant, so the MDF can hold it even though it cannot
// join it to the session's signalling.
func (s *liShipper) checkTag(tagged []byte) bool {
	switch tagged[fseidTagLen] {
	case farForwardDAndDuplicate, farForwardUAndDuplicate, farForwardUEncapAndDuplicate:
	default:
		s.report(x1.NEIssueX3TagInvalid, "content tag carries an unknown forwarding action")

		return false
	}

	if binary.LittleEndian.Uint64(tagged[:fseidTagLen]) == 0 {
		s.report(x1.NEIssueX3TagInvalid, "content tag carries no session correlation")
	}

	return true
}

// reconnect closes the dead X3 egress socket and redials it with capped
// exponential backoff, blocking until the datapath socket is available again.
//
// The egress counts as down for exactly as long as this takes, which is what the
// x3EgressDown probe reports. Nothing else clears it: an element asked while the datapath is
// away says so, and the same element asked after it returns does not.
func (s *liShipper) reconnect() {
	s.egressDown.Store(true)
	if s.sock != nil {
		_ = s.sock.Close()
	}

	s.establish()
}

// establish dials the datapath egress and does not give up.
//
// Separate from reconnect so the *first* dial retries too: a single attempt during
// initialisation that failed would disable content interception for the life of the
// process. The two containers involved start concurrently with no readiness gate, so
// the first attempt is the one most likely to fail.
//
// It tries before it sleeps: a redial after a failure should back off first, but an
// initial dial against a datapath that is already up should not pay 100ms for the
// privilege.
//
// Called only from the shipping loop, which is the sole reader and writer of sock.
func (s *liShipper) establish() {
	delay := minReconnectDelay
	for {
		var d net.Dialer
		sock, err := d.DialContext(context.Background(), "unixpacket", s.sockAddr)
		if err == nil {
			setPuntReadBuffer(sock, s.rcvBuf)
			s.sock = sock
			s.egressDown.Store(false)

			return
		}

		time.Sleep(delay)

		if delay < maxReconnectDelay {
			delay *= 2
		}
	}
}

// shipperPDU frames one teed datapath packet as an X3 content-of-communication
// PDU, labelled with the identity the CC-TF supplied for that session's task. The
// datapath prepends the F-SEID (which selects the task) and the FAR action; the
// action distinguishes the downlink copy — teed after GTP-U encap, so a GTP-U
// packet toward the target — from the uplink copy — decapsulated inner IP from
// the target, or a GTP-U packet toward another UPF where the FAR creates an outer
// header — which sets both the X3 payload format and the direction. Getting this
// wrong would ship an encapsulated copy mislabeled as the target's own IP packet.
func shipperPDU(tagged []byte, task types.InterceptTask, ids *x2x3.Identity) *x2x3.PDU {
	action := tagged[fseidTagLen]
	inner := tagged[liTagLen:]
	pdu := &x2x3.PDU{Type: x2x3.PDUTypeX3}
	// Both identity fields come from the LI_T3 task and neither is derived here:
	// the XID is the warrant's (the task's ProductID, per TS 103 221-1 clause
	// 6.2.1.2, which is what makes the content attributable at all), and the
	// correlation identifier is the value the SMF also put on that session's xIRI,
	// which is what lets the MDF join content to signalling. Deriving either
	// locally is how the two streams came to disagree.
	pdu.XID = task.DeliveryXID().Bytes()
	binary.BigEndian.PutUint64(pdu.CorrelationID[:], task.CorrelationID)

	// The four conditional attributes TS 33.128 table 5.3.3-2 requires of an xCC.
	// Neither target identifier is among them, and neither is sent: this POI is tasked
	// by packet-detection criteria and resolves no subscriber identity, so it has none
	// to report and would have to invent one.
	//
	// The timestamp is the time the xCC is generated, which the table asks for and
	// which is here — so the datapath carries no per-packet timestamp to this point and
	// the BESS tag layout does not grow.
	pdu.Attributes = ids.Attributes(pdu.XID, pdu.CorrelationID, time.Now(), nil, nil)

	l3, format := networkLayerOf(inner)
	pdu.Payload = append([]byte(nil), l3...)
	pdu.Direction = directionOf(action)
	pdu.PayloadFormat = format

	// The downlink copy, and the uplink copy of a FAR toward another UPF, are teed
	// after GTP-U encapsulation, so their network layer is the outer packet carrying
	// the tunnel.
	if isEncapsulatedCopy(action) && format != x2x3.PayloadFormatEthernet {
		pdu.PayloadFormat = x2x3.PayloadFormatGTPU
	}

	return pdu
}

// networkLayerOf returns the packet to carry on X3 and its payload format. The
// BESS pipeline tees a full Ethernet frame — its GTP-U decap leaves the link layer
// intact — whereas X3 carries the network-layer packet, so an L2 header is stripped
// when one is present. A payload that is already a bare IP packet is shipped
// unchanged, and anything unrecognised is shipped whole and labelled Ethernet
// rather than mislabelled as IP.
func networkLayerOf(pkt []byte) ([]byte, x2x3.PayloadFormat) {
	if l3, ok := stripEthernet(pkt); ok {
		return l3, payloadFormatOf(l3)
	}

	if len(pkt) > 0 && (pkt[0]>>4 == 4 || pkt[0]>>4 == 6) {
		return pkt, payloadFormatOf(pkt)
	}

	return pkt, x2x3.PayloadFormatEthernet
}

// isUplinkCopy reports whether a copy is of the target's uplink traffic.
func isUplinkCopy(action byte) bool {
	return action == farForwardUAndDuplicate || action == farForwardUEncapAndDuplicate
}

// isEncapsulatedCopy reports whether a copy was teed after GTP-U encapsulation, so that the
// target's packet is inside the tunnel: every downlink copy, and an uplink copy of a FAR
// toward another UPF. Kept apart from the direction because the two no longer coincide.
func isEncapsulatedCopy(action byte) bool {
	return action == farForwardDAndDuplicate || action == farForwardUEncapAndDuplicate
}

// directionOf maps the datapath forward-and-duplicate action to the X3 direction.
func directionOf(action byte) x2x3.PayloadDirection {
	switch action {
	case farForwardDAndDuplicate:
		return x2x3.DirectionToTarget
	case farForwardUAndDuplicate, farForwardUEncapAndDuplicate:
		return x2x3.DirectionFromTarget
	default:
		return x2x3.DirectionUnknown
	}
}

// stripEthernet removes the Ethernet (and any 802.1Q) header from a teed frame,
// returning the network-layer packet. It reports false when the frame does not
// carry IPv4/IPv6, in which case the caller must not claim an IP payload format.
func stripEthernet(frame []byte) ([]byte, bool) {
	const (
		ethHeaderLen  = 14
		vlanTagLen    = 4
		etherTypeIPv4 = 0x0800
		etherTypeIPv6 = 0x86DD
		etherTypeVLAN = 0x8100
		etherTypeQinQ = 0x88A8
	)

	offset := ethHeaderLen
	for len(frame) > offset {
		etherType := binary.BigEndian.Uint16(frame[offset-2 : offset])
		switch etherType {
		case etherTypeIPv4, etherTypeIPv6:
			return frame[offset:], true
		case etherTypeVLAN, etherTypeQinQ:
			offset += vlanTagLen
		default:
			return frame, false
		}
	}

	return frame, false
}

// payloadFormatOf classifies an inner user-plane packet by IP version.
func payloadFormatOf(pkt []byte) x2x3.PayloadFormat {
	if len(pkt) > 0 && pkt[0]>>4 == 6 {
		return x2x3.PayloadFormatIPv6
	}

	return x2x3.PayloadFormatIPv4
}

// validateLiConfig refuses a content-interception configuration this element cannot
// carry out.
//
// It lives beside the shipper rather than in validateConf for the reason stated there:
// a refusal at configuration-validation time is fatal to the whole process and names
// the field that caused it, which for an LI field is both an outage and a disclosure.
//
// Every field it requires is required because its absence makes interception silently
// impossible rather than degraded — a CC-POI with no triggering interface receives no
// warrant identity and can only produce content no mediation function can attribute.
func validateLiConfig(cfg *LiConfig) error {
	for name, val := range map[string]string{
		"li.x3_sockaddr": cfg.X3SockAddr,
		"li.cert":        cfg.Cert,
		"li.key":         cfg.Key,
		"li.ca_cert":     cfg.CACert,
		// The triggering interface is not optional either: a CC-POI with no LI_T3
		// listener receives no warrant identity, so it can only produce content no
		// mediation function is able to attribute.
		"li.x1_listen": cfg.X1Listen,
		"li.tf_id":     cfg.TFID,
		"li.ne_id":     cfg.NEID,
	} {
		if val == "" {
			return fmt.Errorf("li: %s is required when li is configured", name)
		}
	}

	// An unparseable keepalive timer is rejected rather than treated as the default: a
	// deployment that asked for a timer and silently got another holds tasking on terms
	// nobody chose.
	keepalive, err := keepaliveProcedure(cfg)
	if err != nil {
		return fmt.Errorf("%w (each timer is a positive Go duration, e.g. %q; empty takes the default)", err, "5m")
	}
	// And TIME_P2 long enough to mean what it is for. A triggering function sends
	// keepalives on a cadence it does not let an operator change, so a TIME_P2 shorter than
	// a couple of those reports — and then purges — tasking that is live and being answered
	// for: the procedure firing as a fault rather than as a backstop.
	if keepalive != nil && tooShortTriggerKeepalive(keepalive.TimeP2) {
		return fmt.Errorf("li: keepalive_timeout %s must be at least %s, or tasking a healthy triggering function still holds will be purged",
			keepalive.TimeP2, minTriggerKeepalive)
	}

	return nil
}
