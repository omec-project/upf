// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"testing"
	"time"
)

// Cases where usage reporting is easy to get wrong: a quota counted across threshold
// reports, a volume many thresholds long, a zero time threshold, and a read that outlasts
// the poll interval.

// Threshold reports re-apply the threshold; the quota is counted across them, so it is
// still reached at its volume.
func TestTheQuotaIsReachedAcrossThresholdReports(t *testing.T) {
	u := quotaURR()
	u.triggers.volumeThreshold = true
	u.volumeThreshold = volumeLimit{total: 3 * mb, hasTotal: true}

	s := quotaSession(u)

	var triggers []reportTrigger

	for i := range 4 {
		for _, r := range poll(s, downlink(3*mb), testEpoch.Add(time.Duration(i+1)*time.Second)) {
			triggers = append(triggers, r.trigger)
		}
	}

	// 3, 6 and 9 MB reach the threshold; 12 MB reaches the 10 MB quota too.
	want := []reportTrigger{triggerVOLTH, triggerVOLTH, triggerVOLTH, triggerVOLTH | triggerVOLQU}
	if len(triggers) != len(want) {
		t.Fatalf("reports with triggers %v, expected %v", triggers, want)
	}

	for i := range want {
		if triggers[i] != want[i] {
			t.Fatalf("reports with triggers %v, expected %v", triggers, want)
		}
	}
}

// A volume far past a small threshold makes one report, carrying all of it.
func TestAVolumeManyThresholdsLongMakesOneReport(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1))

	r := poll(s, downlink(1<<40), testEpoch.Add(time.Second))
	if len(r) != 1 || r[0].volume.total != 1<<40 {
		t.Fatalf("expected one report of the whole volume, got %+v", r)
	}
}

// A Time Threshold of zero sets no threshold; it does not fire on every poll.
func TestAZeroTimeThresholdNeverFires(t *testing.T) {
	s := measuredSession(urr{
		urrID:           testURRID,
		measureDuration: true,
		triggers:        urrTriggers{timeThreshold: true},
	})

	for i := range 3 {
		if r := poll(s, someUplink(), testEpoch.Add(time.Duration(i)*time.Hour)); len(r) != 0 {
			t.Fatalf("a zero Time Threshold fired: %+v", r)
		}
	}
}

// deadlineReader records the deadline each read is given.
type deadlineReader struct{ deadline time.Time }

func (r *deadlineReader) ReadUsage(ctx context.Context) (flowMeasureCounts, error) {
	r.deadline, _ = ctx.Deadline()

	return flowMeasureCounts{}, nil
}

// A read is given up before the next poll is due, so slow reads cannot stack up.
func TestAReadEndsBeforeTheNextPoll(t *testing.T) {
	const interval = 5 * time.Second

	reader := &deadlineReader{}
	m := newUsageMonitor(&PFCPNode{ctx: context.Background()}, reader, interval)

	start := time.Now()
	m.poll(start)

	if reader.deadline.IsZero() || !reader.deadline.Before(start.Add(interval)) {
		t.Fatalf("the read's deadline is %v, expected one before the next poll at %v",
			reader.deadline, start.Add(interval))
	}
}
