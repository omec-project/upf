// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/omec-project/util/li/x1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// Content of communication leaves the datapath through a userspace socket, and a
// socket can fill. When it does, the datapath's write fails and the copy is
// discarded before any Go code sees it — so the shipper cannot report a loss it
// never witnessed, and neither can the delivery layer.
//
// That is the whole problem: a throughput ceiling is a capacity fact and can be
// documented, but *silent* loss means a mediation function receives part of the
// ordered content believing it complete, and an agency has no way to know. LI-plane
// faults are reported so that they reach the ADMF rather than failing invisibly.
//
// The only vantage point that can see the loss is the datapath's own accounting.
// Copies are lost in three places on the way out: a tee that cannot make the copy,
// the egress queue, which discards on enqueue when it is full, and the egress port,
// which discards what its write did not take. The queue and the port keep their own
// cumulative discard counts. The tee keeps none, but BESS tracks every connected
// output gate by default, so the copy gate falling behind the original's is the
// tee's loss. The PFCP agent already holds a bessd channel for programming the
// pipeline, so it can read all three and report growth.

const (
	// liPuntPollInterval is how often the egress accounting is read. Loss
	// matters in aggregate rather than per packet — the ADMF needs to know that an
	// interception is under-delivering, not which packet went missing — so this is
	// deliberately slow enough to cost nothing on a busy datapath.
	liPuntPollInterval = 30 * time.Second

	// liX3Port is the port the duplicated copies leave through.
	liX3Port = "liX3"
)

// bessCounters is the slice of the bessd API this monitor needs: two counter
// reads. Narrowed to those so the reporting logic can be tested without a
// datapath, since the logic — what counts as new loss, and what counts as a
// restart rather than a recovery — is where the mistakes would be.
type bessCounters interface {
	// GetPortStats is how the egress port's discards are read.
	GetPortStats(ctx context.Context, in *pb.GetPortStatsRequest, opts ...grpc.CallOption) (*pb.GetPortStatsResponse, error)
	// ModuleCommand is how a module's own accounting is read — Queue.get_status here,
	// which is the only counter that sees a discard on enqueue.
	ModuleCommand(ctx context.Context, in *pb.CommandRequest, opts ...grpc.CallOption) (*pb.CommandResponse, error)
	// GetModuleInfo is how a tee's output gates are read: each carries the packet count
	// of the Track hook BESS attaches to every connected output gate.
	GetModuleInfo(ctx context.Context, in *pb.GetModuleInfoRequest, opts ...grpc.CallOption) (*pb.GetModuleInfoResponse, error)
}

// liQueueModule is the buffering stage in front of the egress port.
const liQueueModule = "liQueue"

// liTeeModules are the pipeline's two tees, downlink and uplink. Each is a Replicate whose
// output gate 0 carries the subscriber's original and gate 1 the copy.
var liTeeModules = [...]string{"liReplicaD", "liReplicaU"}

const (
	teeOriginalGate = 0
	teeCopyGate     = 1

	// teeSkewAllowance is the growth of a tee's shortfall that is reported a poll later
	// rather than at once: one batch, bess::PacketBatch::kMaxBurst (bess/core/pktbatch.h).
	// It is not a bound on how far the copy gate can read ahead -- that skew lowers the
	// shortfall and never reaches this -- but the size of what a baseline taken from such a
	// read is most likely to be off by. See checkTeeCopies.
	teeSkewAllowance = 64

	// trackHookClass is the class of BESS's default gate hook, whose counts these are.
	trackHookClass = "Track"
)

// teeAccount is what the monitor holds about one tee between polls.
type teeAccount struct {
	// original and copied are the gate counts as last read, kept to recognise a reset: a
	// pipeline reloaded, or a Track hook reset, starts both from zero.
	original, copied uint64
	// deficit is the shortfall of copies already accounted for: reported, or present at a
	// reset. Only growth past it is loss this monitor has not yet reported. It starts at
	// zero, so the first poll reports a shortfall already counted, as the other two
	// counters do.
	deficit int64
	// pending is a growth of up to one batch seen on the last poll and not yet reported. It is
	// reported if the next poll still shows it.
	pending bool
}

// liPuntMonitor watches for content discarded between the datapath and the
// shipper.
type liPuntMonitor struct {
	client   bessCounters
	reporter neIssueReporter
	// lost is the egress port's cumulative discard count as last read. Only growth is
	// reported, because a steady figure is one fault, not a fault per poll. It starts at
	// zero, so the first poll reports any discards already counted as growth -- including
	// ones from before this agent started, when the agent restarted and bessd did not.
	lost uint64
	// dropped is the buffering stage's own cumulative discard count, tracked the same way
	// and for the same reason. It is a separate figure rather than added to lost, because
	// the two are different losses at different points and a datapath restart resets them
	// independently.
	dropped uint64
	// tees is each tee's account, indexed as liTeeModules.
	tees [len(liTeeModules)]teeAccount
}

// startLIPuntMonitor begins watching the LI egress accounting. It is silent when
// no reporting channel is configured, because what it detects has nowhere
// else it may legitimately go — a general log is exactly where it must not appear.
func startLIPuntMonitor(client bessCounters, reporter neIssueReporter) {
	if client == nil || reporter == nil {
		return
	}

	m := &liPuntMonitor{client: client, reporter: reporter}
	go m.run()
}

func (m *liPuntMonitor) run() {
	ticker := time.NewTicker(liPuntPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.check()
	}
}

// check reads each tee's gate counts and both discard counters once, and reports any new loss.
//
// Each discard counter is cumulative and read on its own, so traffic passing between the two
// reads changes neither: a discard count grows only when a copy is discarded. That is why the
// port's own count is read rather than inferred from what was handed to the port against
// what it sent — two separate reads of moving packet counters differ by whatever passed
// between them, which on lossless traffic reads as loss.
func (m *liPuntMonitor) check() {
	for i, name := range liTeeModules {
		m.checkTeeCopies(name, &m.tees[i])
	}

	m.checkQueueDrops()
	m.checkPortDrops()
}

// checkTeeCopies reports copies a tee could not make.
//
// Replicate skips a gate when bess::Packet::copy fails -- the packet pool is empty, or the
// packet is in more than one segment -- and counts nothing, so the copy reaches neither the
// queue nor the port and neither of their counts sees it. The subscriber's original still
// goes out on gate 0. The loss is therefore the growth of gate 0's count over gate 1's.
//
// The counts are read while the workers run, and the reads are not atomic with them, but on
// lossless traffic the skew has one direction. Replicate emits each batch's copies before its
// originals, a gate's hook runs when its batch is handed on, in the order the gates first
// received a packet, and bessd reads gate 0 before gate 1. A read can therefore find the copy
// gate ahead and not behind -- how far ahead depends on the batches the workers finish between
// the two hooks and the two reads -- and ahead lowers the shortfall, so it cannot make a loss
// out of lossless traffic. The one batch that can read the other way is one whose first copy
// failed, which is already a loss.
//
// A growth of up to one batch (teeSkewAllowance) is reported a poll later, not at once, and
// only if that poll still shows it; a larger one is reported at once. The wait is not a skew
// filter, since a copy-ahead read never shows growth. It is for a baseline taken from a
// copy-ahead read after a report, which makes the next reads show a growth that persists;
// that can still cost one report of up to a batch the tee did copy. A baseline taken at a
// reset is clamped at zero, so a reload read with the copy gate ahead costs nothing.
//
// Silent where the tee or either gate's Track hook cannot be read -- an older pipeline, or
// one built with the default hooks skipped -- since a count that is not kept is not loss.
func (m *liPuntMonitor) checkTeeCopies(name string, acct *teeAccount) {
	original, copied, ok := m.teeCounts(name)
	if !ok {
		return
	}

	deficit := int64(original) - int64(copied)

	if original < acct.original || copied < acct.copied {
		// Reset by a pipeline reload: a new baseline rather than a recovery. Not below zero: a
		// reset read with the copy gate ahead would otherwise leave a baseline the next
		// unskewed reads grow past.
		*acct = teeAccount{original: original, copied: copied, deficit: max(deficit, 0)}

		return
	}

	acct.original, acct.copied = original, copied

	growth := deficit - acct.deficit

	switch {
	case growth <= 0:
		// Nothing new, or a read that caught the copy gate ahead.
		acct.pending = false

		return
	case growth <= teeSkewAllowance && !acct.pending:
		acct.pending = true

		return
	}

	acct.deficit = deficit
	acct.pending = false

	m.reporter.Notify(x1.NEIssueX3PuntLost,
		"content copies the datapath tee could not make")
}

// teeCounts reads a tee's original and copy gate packet counts, and reports whether both were
// there to read.
func (m *liPuntMonitor) teeCounts(name string) (original, copied uint64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	res, err := m.client.GetModuleInfo(ctx, &pb.GetModuleInfoRequest{Name: name})
	if err != nil || res.GetError() != nil {
		return 0, 0, false
	}

	var haveOriginal, haveCopy bool

	for _, g := range res.GetOgates() {
		if !tracked(g) {
			continue
		}

		switch g.GetOgate() {
		case teeOriginalGate:
			original, haveOriginal = g.GetPkts(), true
		case teeCopyGate:
			copied, haveCopy = g.GetPkts(), true
		}
	}

	return original, copied, haveOriginal && haveCopy
}

// tracked reports whether an output gate's counts are kept: only a gate with a Track hook
// has them, and an untracked gate reads zero.
func tracked(g *pb.GetModuleInfoResponse_OGate) bool {
	for _, h := range g.GetGatehooks() {
		if h.GetClassName() == trackHookClass {
			return true
		}
	}

	return false
}

// checkPortDrops reports growth in the egress port's own discard count.
//
// BESS's PortOut counts a batch's unsent packets as the port's output drops — a full socket,
// no shipper connected, or a port administratively down — so this is the loss on the
// datapath's write, counted where it happens.
//
// Silent when the port cannot be read: the shipper notices its socket has gone and reports
// x3EgressDown, and a second report of the same fact from here would add nothing.
func (m *liPuntMonitor) checkPortDrops() {
	dropped, ok := m.portDropped()
	if !ok {
		return
	}

	if dropped <= m.lost {
		// Either steady — already reported — or reset by a datapath restart, which is a
		// new baseline rather than a recovery.
		m.lost = dropped

		return
	}

	m.lost = dropped
	// NE-level only: how much content was lost, never whose. The ADMF can act on
	// this — reduce the tasking, provision more capacity, or accept the gap — but
	// only if it is told.
	m.reporter.Notify(x1.NEIssueX3PuntLost,
		"content copies discarded at the datapath egress socket")
}

// checkQueueDrops reports growth in the buffering stage's own discard count.
//
// The port's count cannot see this loss: a Queue discards on enqueue, when it is full, and
// those copies never reach the port at all.
//
// Silent where the stage is not a queue, or where its status cannot be read: a pipeline
// whose egress stage is the merge has nothing to ask.
func (m *liPuntMonitor) checkQueueDrops() {
	dropped, ok := m.queueDropped()
	if !ok {
		return
	}

	if dropped <= m.dropped {
		// Either steady — already reported — or reset by a datapath restart, which is a
		// new baseline rather than a recovery.
		m.dropped = dropped

		return
	}

	m.dropped = dropped
	m.reporter.Notify(x1.NEIssueX3PuntLost,
		"content copies discarded by the datapath egress queue")
}

// queueDropped reads the egress queue's cumulative discard count, and reports whether
// there was one to read.
func (m *liPuntMonitor) queueDropped() (uint64, bool) {
	arg, err := anypb.New(&pb.QueueCommandGetStatusArg{})
	if err != nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	res, err := m.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: liQueueModule,
		Cmd:  "get_status",
		Arg:  arg,
	})
	if err != nil || res.GetError() != nil {
		// No queue in this pipeline, or a datapath that will not answer. Neither is a
		// statement about loss.
		return 0, false
	}

	var status pb.QueueCommandGetStatusResponse
	if err := res.GetData().UnmarshalTo(&status); err != nil {
		return 0, false
	}

	return status.GetDropped(), true
}

// portDropped reads the egress port's cumulative discard count, and reports whether
// there was one to read.
func (m *liPuntMonitor) portDropped() (uint64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// Unlike getPortStats, no "Fast" suffix: the LI egress is a plain
	// UnixSocketPort named exactly as the pipeline declares it.
	res, err := m.client.GetPortStats(ctx, &pb.GetPortStatsRequest{Name: liX3Port})
	if err != nil || res.GetError() != nil {
		return 0, false
	}

	return res.GetOut().GetDropped(), true
}
