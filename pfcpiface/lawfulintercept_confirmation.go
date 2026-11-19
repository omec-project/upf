// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"sync/atomic"

	"github.com/wmnsk/go-pfcp/ie"
)

// A batch of datapath writes has two outcomes that must not be collapsed into one: the
// PFCP cause the SMF is told, and whether this element can say the FAR writes were applied.
//
// They differ on exactly one case, and it is the common one. A worker whose RPC did not
// complete -- bessd down, or a context deadline reached before GRPCJoin's own timer --
// learns nothing about what the datapath did with the rule. SendMsgToUPF deliberately
// answers that as accepted, because rejecting it would have the caller forget a session
// whose rules may still be installed. For an interception that leniency is unsafe in the
// other direction: a duplication FAR recorded as programmed drops out of every later
// re-derivation, so no pass retries it, and an accepted warrant produces nothing while
// this element reports itself healthy.
//
// ETSI TS 103 221-1 clause 5.3 requires the element to hold that as a fault rather than a
// warning -- "any issue which loses traffic is categorized as a fault" -- and clause
// 6.5.2.2 names the category: "currently unable to collect traffic but not terminating".
// Clause 5.2.2 gives the element room to resolve it, since an action that cannot complete
// within TIME1 (five seconds by default) is answered "OK - Acknowledged" with a status
// report to follow; the PFCP batch deadline is one second, so the next re-derivation has
// time to confirm or to keep the fault standing.
//
// The flag rides on the batch's own context so that the workers' `done` contract and
// GRPCJoin keep the contract every other caller relies on: this adds a second, out-of-band answer
// rather than changing the one they already give.
type batchConfirmation struct {
	unconfirmed atomic.Bool
}

type batchConfirmationKey struct{}

// withBatchConfirmation attaches a fresh confirmation to one batch's context. Per batch,
// not per bess: SendMsgToUPF may be running concurrently for several sessions.
func withBatchConfirmation(ctx context.Context) (context.Context, *batchConfirmation) {
	batch := &batchConfirmation{}

	return context.WithValue(ctx, batchConfirmationKey{}, batch), batch
}

// noteUnconfirmed records that one FAR write in this batch was neither accepted nor refused.
// Silent when the context carries no confirmation, so a caller that builds its own
// context -- clearState, the slice-meter setup -- needs no change.
func noteUnconfirmed(ctx context.Context) {
	if batch, ok := ctx.Value(batchConfirmationKey{}).(*batchConfirmation); ok {
		batch.unconfirmed.Store(true)
	}
}

// confirmingDatapath is the optional half of the datapath contract: a datapath that can
// distinguish a write it saw acknowledged from one it merely failed to have refused.
//
// Optional, and deliberately not part of the datapath interface, which keeps the
// contract every other caller relies on -- this is a lawful-interception concern and every other caller is
// served by the cause alone. A datapath that does not implement it is taken at its word.
type confirmingDatapath interface {
	sendMsgToUPFConfirmed(
		method upfMsgType, all, updated PacketForwardingRules,
	) (uint8, bool)
}

// sendDuplicationWrite is SendMsgToUPF for a batch an interception depends on: the same
// PFCP cause, and separately whether the datapath acknowledged the FAR writes in it.
//
// Only the BESS datapath can tell the two apart. Any other -- the test fakes, and any
// datapath added later -- is taken at its word, so an accepted batch counts as confirmed
// and every caller behaves as it would with SendMsgToUPF alone.
func (u *upf) sendDuplicationWrite(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	if c, ok := u.datapath.(confirmingDatapath); ok {
		return c.sendMsgToUPFConfirmed(method, all, updated)
	}

	cause := u.SendMsgToUPF(method, all, updated)

	return cause, cause == ie.CauseRequestAccepted
}

// removingDatapath is confirmingDatapath's counterpart for a session's removal: the cause and
// completion SendMsgToUPFWithCompletion gives, and also whether the batch's FAR deletes were
// acknowledged.
//
// A removal needs all three. A FAR delete whose RPC did not complete is answered as accepted and
// finished, which is what lets the caller forget the session -- and with it the only record that
// the rule may still be duplicating. Optional for the same reason confirmingDatapath is.
type removingDatapath interface {
	sendMsgToUPFRemoval(all PacketForwardingRules) (cause uint8, finished, confirmed bool)
}

// sendRemoval deletes a session's rules and answers the cause, whether the batch finished, and
// whether its FAR deletes were confirmed. A datapath that cannot tell is taken at its word, as in
// sendDuplicationWrite: a finished, accepted removal counts as confirmed.
func (u *upf) sendRemoval(all PacketForwardingRules) (uint8, bool, bool) {
	if r, ok := u.datapath.(removingDatapath); ok {
		return r.sendMsgToUPFRemoval(all)
	}

	cause, finished := u.SendMsgToUPFWithCompletion(upfMsgTypeDel, all, PacketForwardingRules{})

	return cause, finished, finished && cause == ie.CauseRequestAccepted
}

// sendRulesRemoval is sendRemoval for rules removed from a session that stays: a modification's
// Remove FAR stage, and the rollback of rules a refused modification created. Those callers do not
// act on completion, so a datapath that cannot confirm is asked through SendMsgToUPF exactly as
// before, and an accepted answer counts as confirmed.
func (u *upf) sendRulesRemoval(rules PacketForwardingRules) (uint8, bool) {
	if r, ok := u.datapath.(removingDatapath); ok {
		cause, _, confirmed := r.sendMsgToUPFRemoval(rules)

		return cause, confirmed
	}

	cause := u.SendMsgToUPF(upfMsgTypeDel, rules, PacketForwardingRules{})

	return cause, cause == ie.CauseRequestAccepted
}
