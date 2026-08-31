// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Open Networking Foundation

package pfcpiface

import (
	"sync/atomic"
	"testing"
	"time"
)

// The channel these tests pass to GRPCJoin is deliberately unbuffered, because that is
// what SendMsgToUPF allocated before this fix: it is the shape in which a stranded
// sender is observable.

func TestGRPCJoinWaitsForEveryCallBeforeReturning(t *testing.T) {
	b := &bess{}

	const calls = 4

	done := make(chan bool)

	var reported atomic.Int32

	// The failure is reported first and the successes arrive later. A join that returns
	// on the first failure leaves the remaining senders blocked forever on a channel
	// nobody will read again, and returns while their writes are still in flight.
	go func() {
		reported.Add(1)
		done <- false
	}()

	for i := 1; i < calls; i++ {
		go func() {
			time.Sleep(20 * time.Millisecond)
			reported.Add(1)
			done <- true
		}()
	}

	if b.GRPCJoin(calls, time.Second, done) {
		t.Error("GRPCJoin reported success for a batch containing a failed call")
	}

	if got := reported.Load(); got != calls {
		t.Errorf("GRPCJoin returned with %d of %d calls still in flight; every call must have reported",
			calls-int(got), calls)
	}
}

func TestGRPCJoinReportsSuccessWhenEveryCallSucceeds(t *testing.T) {
	b := &bess{}

	const calls = 3

	done := make(chan bool)

	for i := 0; i < calls; i++ {
		go func() { done <- true }()
	}

	if !b.GRPCJoin(calls, time.Second, done) {
		t.Error("GRPCJoin reported failure for a batch in which every call succeeded")
	}
}

func TestGRPCJoinReportsFailureWhenAnyCallFails(t *testing.T) {
	b := &bess{}

	const calls = 3

	done := make(chan bool)

	// The failure is reported last, so it cannot be found by returning early.
	for i := 0; i < calls-1; i++ {
		go func() { done <- true }()
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		done <- false
	}()

	if b.GRPCJoin(calls, time.Second, done) {
		t.Error("GRPCJoin reported success for a batch whose last call failed")
	}
}

func TestGRPCJoinReturnsWhenACallNeverReports(t *testing.T) {
	b := &bess{}

	done := make(chan bool)

	go func() { done <- true }()

	start := time.Now()

	if b.GRPCJoin(2, 50*time.Millisecond, done) {
		t.Error("GRPCJoin reported success for a batch one of whose calls never reported")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("GRPCJoin took %v to give up; the deadline must bound it", elapsed)
	}
}

// A rule the worker abandons before reaching the datapath must still report a result.
// Where it reports nothing, the join can never account for it: the batch consumes its
// whole deadline and only then reports a failure whose cause is already known.
func TestAPDRThatCannotBeExpandedStillReportsItsResult(t *testing.T) {
	b := &bess{}

	done := make(chan bool, 1)

	// Both ports as range matches is refused by CreatePortRangeCartesianProduct, so
	// addPDR returns before it uses the datapath client at all.
	p := pdr{}
	p.appFilter.srcPortRange = newRangeMatchPortRange(100, 200)
	p.appFilter.dstPortRange = newRangeMatchPortRange(300, 400)

	b.addPDR(t.Context(), done, p)

	select {
	case ok := <-done:
		if ok {
			t.Error("a PDR that was never programmed was reported as done")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("addPDR reported nothing for a rule it abandoned; the batch cannot complete")
	}
}
