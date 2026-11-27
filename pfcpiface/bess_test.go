// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Open Networking Foundation

package pfcpiface

import (
	"sync"
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

	completed, succeeded := b.GRPCJoin(calls, time.Second, done)
	if !completed {
		t.Error("GRPCJoin reported a batch as incomplete when every call reported")
	}

	if succeeded {
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

	if completed, succeeded := b.GRPCJoin(calls, time.Second, done); !completed || !succeeded {
		t.Errorf("GRPCJoin() = (%t, %t) for a batch in which every call succeeded, want (true, true)",
			completed, succeeded)
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

	if completed, succeeded := b.GRPCJoin(calls, time.Second, done); !completed || succeeded {
		t.Errorf("GRPCJoin() = (%t, %t) for a batch whose last call failed, want (true, false)",
			completed, succeeded)
	}
}

func TestGRPCJoinReturnsWhenACallNeverReports(t *testing.T) {
	b := &bess{}

	done := make(chan bool)

	go func() { done <- true }()

	start := time.Now()

	if completed, succeeded := b.GRPCJoin(2, 50*time.Millisecond, done); completed || succeeded {
		t.Errorf("GRPCJoin() = (%t, %t) for a batch one of whose calls never reported, want (false, false)",
			completed, succeeded)
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

// TestASecondBatchCannotBeOvertakenByTheFirst is the ordering half, which lawful interception
// depends on.
//
// A tasking pass and the session path both push the same FAR. They are ordered against each other
// by the element, and that ordering only means anything if a batch's writes have actually landed
// when SendMsgToUPF returns. While GRPCJoin returned on the first failure with the rest of the
// batch still in flight, an earlier push could be overtaken by a later one: the element records
// the value it pushed second, and the datapath holds the value that arrived second — which is the
// first. The record and the datapath then disagree, silently, which is the whole subject of this
// change.
//
// Asserted at the join, because that is where the guarantee lives: after it returns, no worker of
// that batch may still write.
func TestASecondBatchCannotBeOvertakenByTheFirst(t *testing.T) {
	t.Parallel()

	b := &bess{}

	var (
		mu      sync.Mutex
		arrived []string
	)

	record := func(what string) {
		mu.Lock()
		arrived = append(arrived, what)
		mu.Unlock()
	}

	// The first batch: one worker fails immediately, two more are slow. Before the fix the join
	// returned on the failure and the slow two landed later — after the second batch.
	first := make(chan bool, 3)

	go func() { record("first"); first <- false }()

	for range 2 {
		go func() {
			time.Sleep(20 * time.Millisecond)
			record("first")
			first <- true
		}()
	}

	b.GRPCJoin(3, time.Second, first)

	// The second batch is issued only now, as the element issues its next push.
	second := make(chan bool, 1)

	go func() { record("second"); second <- true }()

	b.GRPCJoin(1, time.Second, second)

	// Long enough for any straggler from the first batch to have landed. With the join draining
	// its batch there are none — that is the property. Without it the two slow workers arrive
	// here, after the second batch, and the assertion below sees them; a test that read the
	// record immediately would return before they arrived and pass against the defect.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var seenSecond bool

	for i, what := range arrived {
		if what == "second" {
			seenSecond = true

			continue
		}

		if seenSecond {
			t.Fatalf("a write from the first batch arrived at position %d, after the second "+
				"batch's: the element pushed one value and the datapath kept the other, and "+
				"nothing on either side can see the disagreement. Order: %v", i, arrived)
		}
	}
}
