// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// TestADroppedCopyIsReportedAsALossAndNotAsUnreachability is the CC-POI's half of the
// unreported drop, asserted where the decision is made.
//
// A partial write costs one content copy only where the library's own resend of that unit
// does not land either — it resends it whole on the fresh connection, which recovers the
// ordinary case. What is left is a genuine loss to a destination that took the rest of the
// batch, and the library reports it as ErrUnitDropped precisely so it is not mistaken for
// unreachability: a healthy mediation function reported unreachable would have the watcher
// raise a fault about a working peer and retract it on the next send.
//
// If both hooks — the delivery one and the keepalive's — were a bare nudge, the error would be
// discarded and the watcher would sample a destination it correctly considers reachable. The
// loss would be reported by nothing at all.
//
// The transport that produces the error is tested where it lives, in li/x2x3's partial-write
// suite, which drives a real socket. What this element does with it is this hook.
func TestADroppedCopyIsReportedAsALossAndNotAsUnreachability(t *testing.T) {
	rec := &recordingReporter{}
	s := &liShipper{reporter: rec, senders: make(map[string]x2x3.Sender)}

	// The error as the library returns it: wrapped, because a caller must match it with
	// errors.Is rather than by comparison.
	s.reportDeliveryError(fmt.Errorf("%w: send to 192.0.2.1:42069", x2x3.ErrUnitDropped))

	if !slices.Contains(rec.reported(), x1.NEIssueX3DeliveryLost) {
		t.Errorf("a content copy was dropped on the way to a reachable mediation function and "+
			"reported as %v, want %s: the loss the library correctly stops mis-reporting as "+
			"unreachability was then reported nowhere at all",
			rec.reported(), x1.NEIssueX3DeliveryLost)
	}
}

// And the other direction, which keeps the fix from turning every delivery failure into a
// product-loss report: an ordinary transport failure says the destination is not working,
// which the watcher reports at the scope that names it.
func TestAnOrdinaryDeliveryFailureIsNotReportedAsALostCopy(t *testing.T) {
	rec := &recordingReporter{}
	s := &liShipper{reporter: rec, senders: make(map[string]x2x3.Sender)}

	s.reportDeliveryError(errors.New("dial tcp 192.0.2.1:42069: connection refused"))

	if slices.Contains(rec.reported(), x1.NEIssueX3DeliveryLost) {
		t.Errorf("a connection failure was reported as lost content: %v", rec.reported())
	}
}

// TestADroppedRunOfCopiesIsReportedWithItsCount: a resend that does not land loses every copy
// from the one the write stopped in to the end of its batch, and the library counts them. The
// report must say how many, or the agency's gap is larger than this element's account of it.
func TestADroppedRunOfCopiesIsReportedWithItsCount(t *testing.T) {
	rec := &recordingReporter{}
	s := &liShipper{reporter: rec, senders: make(map[string]x2x3.Sender)}

	s.reportDeliveryError(&x2x3.DeliveryError{
		Lost: 7,
		Err:  fmt.Errorf("%w: send to 192.0.2.1:42069", x2x3.ErrUnitDropped),
	})

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.descriptions) != 1 || !strings.Contains(rec.descriptions[0], "7 content copies") {
		t.Errorf("the loss was reported as %q, want it to say how many copies it cost", rec.descriptions)
	}
}
