// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"slices"
	"testing"
	"time"
)

// What a URR measures and when it reports, against TS 29.244 clause 5.2.2. The URR's
// usage is credited the way a poll credits it, and its reports are taken the way a poll
// takes them; delivery is settled by hand.

const (
	mb = 1000 * 1000
)

// measuredSession is a session with one PDR measured by the URR.
func measuredSession(u urr) *PFCPSession {
	s := &PFCPSession{}
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{u.urrID}, hasURRIDs: true})
	s.CreateURR(u, testEpoch)

	return s
}

func downlink(bytes uint64) volumeCounts {
	return volumeCounts{total: bytes, downlink: bytes}
}

func uplink(bytes uint64) volumeCounts {
	return volumeCounts{total: bytes, uplink: bytes}
}

// someUplink is a little uplink traffic, enough to start time metering.
func someUplink() volumeCounts {
	return volumeCounts{total: 1, uplink: 1}
}

// poll credits v and takes the session's due reports at, settling them as delivered.
func poll(s *PFCPSession, v volumeCounts, at time.Time) []*usageReport {
	s.addUsage(downlinkPDRID, v, at)
	reports := s.dueReports(at)
	s.settleReports(reports, reportDelivered)

	return reports
}

func volumeThresholdURR(threshold uint64) urr {
	return urr{
		urrID:           testURRID,
		measureVolume:   true,
		triggers:        urrTriggers{volumeThreshold: true},
		volumeThreshold: volumeLimit{total: threshold, hasTotal: true},
	}
}

// Clause 5.2.2.3.1: each report carries the usage since
// the previous one, and the first has UR-SEQN 0.
func TestReportsCarryTheUsageSinceTheLastReport(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	first := poll(s, downlink(4*mb), testEpoch.Add(time.Second))
	second := poll(s, downlink(1*mb), testEpoch.Add(2*time.Second))

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one report per poll, got %d and %d", len(first), len(second))
	}

	if first[0].volume != downlink(4*mb) || second[0].volume != downlink(1*mb) {
		t.Fatalf("the reports carry %+v and %+v, expected 4 MB and 1 MB downlink",
			first[0].volume, second[0].volume)
	}

	if first[0].seq != 0 || second[0].seq != 1 {
		t.Fatalf("the reports have UR-SEQN %d and %d, expected 0 and 1", first[0].seq, second[0].seq)
	}

	if !first[0].start.Equal(testEpoch) || !second[0].start.Equal(first[0].end) {
		t.Fatalf("the reports cover %v-%v and %v-%v; each should start where the last ended",
			first[0].start, first[0].end, second[0].start, second[0].end)
	}
}

// A threshold report re-applies the threshold; a report for another trigger leaves it
// counting from where it was (clause 5.2.2.3.1).
func TestOnlyAThresholdReportReappliesTheVolumeThreshold(t *testing.T) {
	u := volumeThresholdURR(10 * mb)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	periodic := poll(s, downlink(6*mb), testEpoch.Add(time.Minute))
	if len(periodic) != 1 || periodic[0].trigger != triggerPERIO {
		t.Fatalf("expected one periodic report, got %+v", periodic)
	}

	threshold := poll(s, downlink(4*mb), testEpoch.Add(time.Minute+time.Second))
	if len(threshold) != 1 || threshold[0].trigger != triggerVOLTH {
		t.Fatalf("expected the threshold to be reached at 10 MB across the periodic report, got %+v", threshold)
	}

	if threshold[0].volume.total != 4*mb {
		t.Fatalf("the threshold report carries %d bytes, expected the 4 MB since the periodic report",
			threshold[0].volume.total)
	}

	if again := poll(s, downlink(9*mb), testEpoch.Add(time.Minute+2*time.Second)); len(again) != 0 {
		t.Fatalf("the threshold fired again at 9 MB after it was re-applied: %+v", again)
	}
}

// A Volume Threshold the control plane sends again is counted from the URR's last
// report (TS 29.244 clause 5.2.2.3.1 NOTE 1), not from the last threshold report: sent
// after a periodic report, it fires a whole threshold later.
func TestAResentThresholdIsCountedFromTheLastReport(t *testing.T) {
	u := volumeThresholdURR(10 * mb)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	if periodic := poll(s, downlink(6*mb), testEpoch.Add(time.Minute)); len(periodic) != 1 {
		t.Fatalf("expected one periodic report, got %+v", periodic)
	}

	if err := s.UpdateURR(volumeThresholdURR(10*mb), urrIEs{volumeThreshold: true}, testEpoch); err != nil {
		t.Fatalf("Update URR refused: %v", err)
	}

	if early := poll(s, downlink(6*mb), testEpoch.Add(time.Minute+time.Second)); len(early) != 0 {
		t.Fatalf("the re-sent threshold fired 6 MB after the last report: %+v", early)
	}

	if due := poll(s, downlink(4*mb), testEpoch.Add(time.Minute+2*time.Second)); len(due) != 1 ||
		due[0].trigger != triggerVOLTH || due[0].volume.total != 10*mb {
		t.Fatalf("expected a threshold report of the 10 MB since the last report, got %+v", due)
	}
}

// Clause 5.2.2.3.1: all three counters are reported,
// zero or not.
func TestAVolumeReportCarriesEveryCounter(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	r := poll(s, downlink(1*mb), testEpoch.Add(time.Second))
	if len(r) != 1 || !r[0].hasVolume {
		t.Fatalf("expected one volume report, got %+v", r)
	}

	if r[0].volume != (volumeCounts{total: 1 * mb, uplink: 0, downlink: 1 * mb}) {
		t.Fatalf("the report carries %+v", r[0].volume)
	}
}

// "Periodic" is a report each time the measurement period is over, whatever else was
// reported in it; a poll that comes late sends one report for the periods it missed.
func TestPeriodicReportsFollowTheMeasurementPeriod(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 10 * time.Second,
	})

	var fired []time.Duration

	for _, at := range []time.Duration{0, 5, 10, 15, 20, 45, 50} {
		if r := poll(s, someUplink(), testEpoch.Add(at*time.Second)); len(r) > 0 {
			fired = append(fired, at)
		}
	}

	want := []time.Duration{10, 20, 45, 50}
	if len(fired) != len(want) {
		t.Fatalf("periodic reports at %v s, expected %v s", fired, want)
	}

	for i := range want {
		if fired[i] != want[i] {
			t.Fatalf("periodic reports at %v s, expected %v s", fired, want)
		}
	}
}

// Time is metered from the first traffic (clause 5.2.2.2.1), and Duration Measurement
// counts whole seconds without losing the parts it cannot carry.
func TestTimeIsMeteredFromTheFirstTrafficAndNoSecondIsLost(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 1500 * time.Millisecond,
	})

	poll(s, volumeCounts{}, testEpoch) // arms the period, no traffic

	var total time.Duration

	for at := 1500 * time.Millisecond; at <= 6*time.Second; at += 1500 * time.Millisecond {
		for _, r := range poll(s, someUplink(), testEpoch.Add(at)) {
			total += r.duration
		}
	}

	// Traffic was first seen at 1.5 s, so 4.5 s were metered by 6 s, of which the reports
	// carry the 4 whole seconds.
	if total != 4*time.Second {
		t.Fatalf("the reports carry %v of duration, expected 4s", total)
	}
}

func TestTheTimeThresholdFiresAfterTheTimeItSets(t *testing.T) {
	s := measuredSession(urr{
		urrID:           testURRID,
		measureDuration: true,
		triggers:        urrTriggers{timeThreshold: true},
		timeThreshold:   time.Minute,
	})

	if r := poll(s, someUplink(), testEpoch.Add(time.Hour)); len(r) != 0 {
		t.Fatalf("the time threshold fired at the first traffic: %+v", r)
	}

	if r := poll(s, someUplink(), testEpoch.Add(time.Hour+time.Minute)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report a minute after the first traffic, got %+v", r)
	}
}

// bothThresholdsURR measures volume and time, with a Volume Threshold of 10 MB and a
// Time Threshold of a minute.
func bothThresholdsURR() urr {
	u := volumeThresholdURR(10 * mb)
	u.measureDuration = true
	u.triggers.timeThreshold = true
	u.timeThreshold = time.Minute

	return u
}

// A report triggered by one threshold re-applies all of them (clause 5.2.2.3.1): the
// other threshold counts afresh from the report.
func TestAThresholdReportReappliesEveryThreshold(t *testing.T) {
	s := measuredSession(bothThresholdsURR())

	poll(s, downlink(8*mb), testEpoch) // metering starts

	if r := poll(s, volumeCounts{}, testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report a minute after the first traffic, got %+v", r)
	}

	// The time threshold report re-applied the Volume Threshold, so the 8 MB before it no
	// longer count toward it.
	if r := poll(s, downlink(9*mb), testEpoch.Add(61*time.Second)); len(r) != 0 {
		t.Fatalf("the volume threshold fired at 9 MB after a time threshold report: %+v", r)
	}

	if r := poll(s, downlink(1*mb), testEpoch.Add(62*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected a volume threshold report at 10 MB after the time threshold report, got %+v", r)
	}

	// And the volume threshold report re-applied the Time Threshold.
	if r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Minute)); len(r) != 0 {
		t.Fatalf("the time threshold fired a minute after the last time threshold report, not the last threshold report: %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Minute+2*time.Second)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report a minute after the volume threshold report, got %+v", r)
	}
}

// A refused threshold report did not re-apply the thresholds: they count on from where
// they were, the time threshold too.
func TestARefusedThresholdReportRestoresEveryThreshold(t *testing.T) {
	s := measuredSession(bothThresholdsURR())

	poll(s, downlink(1*mb), testEpoch) // metering starts

	s.addUsage(downlinkPDRID, downlink(9*mb), testEpoch.Add(30*time.Second))

	sent := s.dueReports(testEpoch.Add(30 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLTH {
		t.Fatalf("expected a volume threshold report, got %+v", sent)
	}

	s.settleReports(sent, reportRejected)

	// The refused report's 10 MB count toward the Volume Threshold again, and the Time
	// Threshold runs from the first traffic again: both are due.
	r := poll(s, volumeCounts{}, testEpoch.Add(time.Minute))
	if len(r) != 1 || r[0].trigger != triggerVOLTH|triggerTIMTH || r[0].volume.total != 10*mb {
		t.Fatalf("expected one report of both thresholds carrying the refused 10 MB, got %+v", r)
	}
}

// The Volume Threshold a refused report restores includes what it had counted before
// the report: a periodic report does not re-apply it.
func TestARefusedThresholdReportRestoresTheVolumeBeforeAPeriodicReport(t *testing.T) {
	u := volumeThresholdURR(10 * mb)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	if r := poll(s, downlink(6*mb), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch.Add(61*time.Second))

	sent := s.dueReports(testEpoch.Add(61 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLTH || sent[0].volume.total != 4*mb {
		t.Fatalf("expected a volume threshold report of the 4 MB since the periodic one, got %+v", sent)
	}

	s.settleReports(sent, reportRejected)

	// The threshold is still reached by the 10 MB it had counted.
	if r := poll(s, volumeCounts{}, testEpoch.Add(62*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected the volume threshold report to be due again, got %+v", r)
	}
}

// periodicVolumeThresholdURR has a Volume Threshold of 10 MB and a period of a minute.
func periodicVolumeThresholdURR() urr {
	u := volumeThresholdURR(10 * mb)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	return u
}

// A Volume Threshold sent while a report is outstanding, and the report then refused, is
// counted from the last report the control plane counted (clause 5.2.2.3.1 NOTE 1): not
// from the refused one, nor restored to what the refused one re-applied.
func TestAVolumeThresholdSentBeforeARefusalCountsFromTheLastCountedReport(t *testing.T) {
	t.Run("a refused periodic report", func(t *testing.T) {
		s := measuredSession(periodicVolumeThresholdURR())

		poll(s, downlink(0), testEpoch) // arms the period

		s.addUsage(downlinkPDRID, downlink(6*mb), testEpoch.Add(time.Minute))
		sent := s.dueReports(testEpoch.Add(time.Minute))

		if err := s.UpdateURR(volumeThresholdURR(10*mb), urrIEs{volumeThreshold: true}, testEpoch); err != nil {
			t.Fatalf("UpdateURR: %v", err)
		}

		s.settleReports(sent, reportRejected)

		// The refused 6 MB count toward the new threshold.
		if r := poll(s, downlink(4*mb), testEpoch.Add(61*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
			t.Fatalf("expected a volume threshold report at 10 MB since the last counted report, got %+v", r)
		}
	})

	t.Run("a refused threshold report", func(t *testing.T) {
		s := measuredSession(periodicVolumeThresholdURR())

		poll(s, downlink(0), testEpoch) // arms the period

		if r := poll(s, downlink(6*mb), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
			t.Fatalf("expected a periodic report, got %+v", r)
		}

		s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch.Add(61*time.Second))
		sent := s.dueReports(testEpoch.Add(61 * time.Second))

		if err := s.UpdateURR(volumeThresholdURR(10*mb), urrIEs{volumeThreshold: true}, testEpoch); err != nil {
			t.Fatalf("UpdateURR: %v", err)
		}

		s.addUsage(downlinkPDRID, downlink(1*mb), testEpoch.Add(62*time.Second))
		s.settleReports(sent, reportRejected)

		// 5 MB since the periodic report: the new threshold is not reached.
		if r := poll(s, volumeCounts{}, testEpoch.Add(63*time.Second)); len(r) != 0 {
			t.Fatalf("the new threshold fired at 5 MB since the last counted report: %+v", r)
		}

		if r := poll(s, downlink(5*mb), testEpoch.Add(64*time.Second)); len(r) != 1 || r[0].volume.total != 10*mb {
			t.Fatalf("expected a report of 10 MB once the new threshold is reached, got %+v", r)
		}
	})
}

// A threshold sent before a report the control plane counted is not new to a later
// refusal: the refused report's thresholds are restored as they were.
func TestAThresholdSentBeforeACountedReportIsRestoredOnALaterRefusal(t *testing.T) {
	s := measuredSession(periodicVolumeThresholdURR())

	poll(s, downlink(0), testEpoch) // arms the period

	if err := s.UpdateURR(volumeThresholdURR(10*mb), urrIEs{volumeThreshold: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if r := poll(s, downlink(6*mb), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch.Add(61*time.Second))
	s.settleReports(s.dueReports(testEpoch.Add(61*time.Second)), reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(62*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected the refused volume threshold report to be due again, got %+v", r)
	}
}

// The same holds for a Time Threshold.
func TestATimeThresholdSentBeforeARefusalCountsFromTheLastCountedReport(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{timeThreshold: true, periodic: true},
		timeThreshold:     time.Minute,
		measurementPeriod: 30 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	if r := poll(s, volumeCounts{}, testEpoch.Add(30*time.Second)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	sent := s.dueReports(testEpoch.Add(time.Minute))
	if len(sent) != 1 || sent[0].trigger&triggerTIMTH == 0 {
		t.Fatalf("expected a time threshold report, got %+v", sent)
	}

	if err := s.UpdateURR(urr{urrID: testURRID, timeThreshold: 100 * time.Second},
		urrIEs{timeThreshold: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	s.settleReports(sent, reportRejected)

	// Counted from the periodic report at 30 s, the new threshold is reached at 130 s.
	for _, at := range []time.Duration{90 * time.Second, 100 * time.Second, 120 * time.Second} {
		for _, r := range poll(s, volumeCounts{}, testEpoch.Add(at)) {
			if r.trigger&triggerTIMTH != 0 {
				t.Fatalf("the new time threshold fired at %v: %+v", at, r)
			}
		}
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(130*time.Second)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report 100 s after the last counted report, got %+v", r)
	}
}

// A refused report returns the part of a second its Duration Measurement carried, so no
// metered time is lost.
func TestARefusedReportReturnsItsDurationCarry(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 10 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	first := poll(s, volumeCounts{}, testEpoch.Add(10900*time.Millisecond))
	if len(first) != 1 || first[0].duration != 10*time.Second {
		t.Fatalf("expected a report of 10 s, got %+v", first)
	}

	refused := s.dueReports(testEpoch.Add(20200 * time.Millisecond))
	s.settleReports(refused, reportRejected)

	// 19.3 s since the first report, and the 0.9 s it carried: 20 whole seconds.
	next := poll(s, volumeCounts{}, testEpoch.Add(30200*time.Millisecond))
	if len(next) != 1 || next[0].duration != 20*time.Second {
		t.Fatalf("expected the next report to carry 20 s, got %+v", next)
	}
}

// A period turned off and on again by Update URR starts afresh, not from where it was.
func TestAPeriodTurnedOnAgainStartsAfresh(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: time.Minute,
	})

	poll(s, downlink(0), testEpoch) // arms the period

	if err := s.UpdateURR(urr{urrID: testURRID}, urrIEs{reportingTriggers: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if err := s.UpdateURR(urr{urrID: testURRID, triggers: urrTriggers{periodic: true}},
		urrIEs{reportingTriggers: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if r := poll(s, downlink(1), testEpoch.Add(5*time.Minute)); len(r) != 0 {
		t.Fatalf("the period turned on again fired at once: %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(6*time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report a period after it was turned on again, got %+v", r)
	}
}

// A Time Threshold the control plane sends is counted from the URR's last report (clause
// 5.2.2.3.1 NOTE 1), as a Volume Threshold is.
func TestAnUpdatedTimeThresholdIsCountedFromTheLastReport(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{timeThreshold: true, periodic: true},
		timeThreshold:     time.Hour,
		measurementPeriod: time.Minute,
	})

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	if r := poll(s, volumeCounts{}, testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	// Sent 20 s after the report, the threshold counts those 20 s.
	poll(s, volumeCounts{}, testEpoch.Add(80*time.Second))

	if err := s.UpdateURR(urr{urrID: testURRID, timeThreshold: 30 * time.Second},
		urrIEs{timeThreshold: true}, testEpoch.Add(80*time.Second)); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(89*time.Second)); len(r) != 0 {
		t.Fatalf("the new time threshold was counted from before the last report: %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(90*time.Second)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report 30 s after the last report, got %+v", r)
	}
}

// A report that may not have arrived is sent again as it was, with its UR-SEQN, so a
// control plane that lost only the answer counts it once; until it is delivered the URR
// starts no other report, and its usage goes into the one after.
func TestAnUndeliveredReportIsSentAgainUnchanged(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	s.addUsage(downlinkPDRID, downlink(3*mb), testEpoch.Add(time.Second))

	sent := s.dueReports(testEpoch.Add(time.Second))
	if len(sent) != 1 {
		t.Fatalf("expected one report, got %+v", sent)
	}

	if again := s.dueReports(testEpoch.Add(2 * time.Second)); len(again) != 0 {
		t.Fatalf("a report being sent was sent again before it settled: %+v", again)
	}

	s.settleReports(sent, reportUndelivered)
	s.addUsage(downlinkPDRID, downlink(1*mb), testEpoch.Add(3*time.Second))

	resent := s.dueReports(testEpoch.Add(3 * time.Second))
	if len(resent) != 1 || *resent[0] != *sent[0] {
		t.Fatalf("the undelivered report was not sent again unchanged: sent %+v, then %+v", sent, resent)
	}

	s.settleReports(resent, reportDelivered)

	next := s.dueReports(testEpoch.Add(4 * time.Second))
	if len(next) != 1 || next[0].seq != sent[0].seq+1 || next[0].volume.total != 1*mb {
		t.Fatalf("expected the next report to carry the 1 MB measured meanwhile, got %+v", next)
	}
}

// A refused report was counted for nothing, so its usage goes into the next report.
func TestARejectedReportsUsageGoesIntoTheNextReport(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	s.addUsage(downlinkPDRID, downlink(3*mb), testEpoch.Add(time.Second))
	sent := s.dueReports(testEpoch.Add(time.Second))
	s.settleReports(sent, reportRejected)

	s.addUsage(downlinkPDRID, downlink(1), testEpoch.Add(2*time.Second))

	next := s.dueReports(testEpoch.Add(2 * time.Second))
	if len(next) != 1 || next[0].volume.total != 3*mb+1 || !next[0].start.Equal(sent[0].start) {
		t.Fatalf("expected the next report to carry the refused 3 MB from its start, got %+v", next)
	}

	if next[0].seq != sent[0].seq+1 {
		t.Fatalf("the next report has UR-SEQN %d, expected %d", next[0].seq, sent[0].seq+1)
	}
}

// A report that settles after its URR was removed or replaced changes nothing.
func TestALateSettlementOfAnotherReportIsIgnored(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	s.addUsage(downlinkPDRID, downlink(3*mb), testEpoch.Add(time.Second))
	sent := s.dueReports(testEpoch.Add(time.Second))

	stale := *sent[0]

	s.settleReports([]*usageReport{&stale}, reportDelivered)

	if s.urrs[0].usage.outstanding == nil {
		t.Fatal("a settlement of a report the URR is not waiting on cleared its outstanding report")
	}
}

// A URR removed and created again under its ID numbers its reports from 0 again. A late
// answer to the old URR's report must not settle the new URR's.
func TestALateSettlementDoesNotReachAURRCreatedAgain(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1 * mb))

	s.addUsage(downlinkPDRID, downlink(3*mb), testEpoch.Add(time.Second))
	old := s.dueReports(testEpoch.Add(time.Second))

	if _, err := s.RemoveURR(testURRID); err != nil {
		t.Fatal(err)
	}

	s.CreateURR(volumeThresholdURR(1*mb), testEpoch.Add(2*time.Second))
	s.addUsage(downlinkPDRID, downlink(2*mb), testEpoch.Add(3*time.Second))

	current := s.dueReports(testEpoch.Add(3 * time.Second))
	if len(current) != 1 || current[0].seq != old[0].seq {
		t.Fatalf("expected the new URR's first report to share the old one's UR-SEQN, got %+v", current)
	}

	s.settleReports(old, reportDelivered)

	if s.urrs[0].usage.outstanding != current[0] {
		t.Fatal("the old URR's late answer settled the new URR's report")
	}
}

// An Update URR's Measurement Period replaces the running one, which then does not fire;
// the new period runs from the next poll.
func TestAnUpdatedMeasurementPeriodReplacesTheRunningOne(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 10 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // arms the 10 s period

	if err := s.UpdateURR(urr{urrID: testURRID, measurementPeriod: 30 * time.Second},
		urrIEs{measurementPeriod: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	var fired []time.Duration

	for _, at := range []time.Duration{10, 39, 40} {
		if r := poll(s, someUplink(), testEpoch.Add(at*time.Second)); len(r) > 0 {
			fired = append(fired, at)
		}
	}

	if len(fired) != 1 || fired[0] != 40 {
		t.Fatalf("periodic reports at %v s, expected one at 40 s: 30 s from the poll after the update", fired)
	}
}

// An Update URR whose Reporting Triggers leave one out stops that trigger's reports. The
// URR keeps measuring, and a report once the trigger is back carries all of it.
func TestAnUpdateURRThatDropsATriggerStopsItsReports(t *testing.T) {
	s := measuredSession(volumeThresholdURR(1000))

	if err := s.UpdateURR(urr{urrID: testURRID}, urrIEs{reportingTriggers: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if r := poll(s, downlink(1500), testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a volume threshold report after VOLTH was dropped: %+v", r)
	}

	if err := s.UpdateURR(urr{urrID: testURRID, triggers: urrTriggers{volumeThreshold: true}},
		urrIEs{reportingTriggers: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Second))
	if len(r) != 1 || r[0].trigger != triggerVOLTH || r[0].volume.total != 1500 {
		t.Fatalf("expected a volume threshold report of the 1500 bytes once VOLTH was back, got %+v", r)
	}
}

// An Update URR's Time Threshold is counted against the time already metered: a new
// threshold is considered against the ongoing measurement (clause 5.2.2.3.1).
func TestAnUpdatedTimeThresholdCountsFromTheSameStart(t *testing.T) {
	s := measuredSession(urr{
		urrID:           testURRID,
		measureDuration: true,
		triggers:        urrTriggers{timeThreshold: true},
		timeThreshold:   time.Minute,
	})

	poll(s, someUplink(), testEpoch) // metering starts

	if err := s.UpdateURR(urr{urrID: testURRID, timeThreshold: 10 * time.Second},
		urrIEs{timeThreshold: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	if r := poll(s, someUplink(), testEpoch.Add(9*time.Second)); len(r) != 0 {
		t.Fatalf("the new time threshold fired early: %+v", r)
	}

	if r := poll(s, someUplink(), testEpoch.Add(10*time.Second)); len(r) != 1 || r[0].trigger != triggerTIMTH {
		t.Fatalf("expected a time threshold report 10 s after metering started, got %+v", r)
	}
}

// setMeasurement sends an Update URR that changes only the Measurement Method.
func setMeasurement(t *testing.T, s *PFCPSession, volume, duration bool, at time.Time) {
	t.Helper()

	if err := s.UpdateURR(urr{urrID: testURRID, measureVolume: volume, measureDuration: duration},
		urrIEs{measurementMethod: true}, at); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}
}

// The time duration measurement is off is not metered: turned off at 10 s and on again
// at 100 s, it bills the 10 s before and the 10 s after, not 110 s.
func TestTimeIsNotMeteredWhileDurationMeasurementIsOff(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 110 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	setMeasurement(t, s, true, false, testEpoch.Add(10*time.Second))
	setMeasurement(t, s, false, true, testEpoch.Add(100*time.Second))

	if r := poll(s, someUplink(), testEpoch.Add(100*time.Second)); len(r) != 0 {
		t.Fatalf("expected no report before the period ends, got %+v", r)
	}

	r := poll(s, volumeCounts{}, testEpoch.Add(110*time.Second))
	if len(r) != 1 || r[0].duration != 20*time.Second {
		t.Fatalf("expected a report of 20 s, got %+v", r)
	}
}

// No time is metered while no PDR measures a URR, so its Time Threshold does not fire on
// it. The time metered before goes in its termination report, and metering starts again at
// the first traffic a PDR credits to it.
func TestTimeIsNotMeteredWhileNoPDRMeasuresTheURR(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true, timeThreshold: true},
		measurementPeriod: 100 * time.Second,
		timeThreshold:     50 * time.Second,
	})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	if r := s.reportDissociated(before, testEpoch.Add(10*time.Second)); len(r) != 1 || r[0].duration != 10*time.Second {
		t.Fatalf("expected a termination report of the 10 s metered before, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(60*time.Second)); len(r) != 0 {
		t.Fatalf("expected no report while no PDR measures the URR, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(100*time.Second)); len(r) != 1 || r[0].duration != 0 {
		t.Fatalf("expected a periodic report with no time metered, got %+v", r)
	}

	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})
	poll(s, volumeCounts{}, testEpoch.Add(150*time.Second))
	poll(s, someUplink(), testEpoch.Add(160*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(200*time.Second)); len(r) != 1 || r[0].duration != 40*time.Second {
		t.Fatalf("expected a report of the 40 s since traffic resumed, got %+v", r)
	}
}

// Time metered before duration measurement is turned off goes into the next report
// that measures duration, past a report that does not.
func TestTimeMeteredBeforeDurationMeasurementIsTurnedOffIsKept(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 50 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	setMeasurement(t, s, true, false, testEpoch.Add(10*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(50*time.Second)); len(r) != 1 || r[0].hasDuration {
		t.Fatalf("expected a report without duration, got %+v", r)
	}

	setMeasurement(t, s, false, true, testEpoch.Add(60*time.Second))
	poll(s, someUplink(), testEpoch.Add(60*time.Second))

	r := poll(s, volumeCounts{}, testEpoch.Add(100*time.Second))
	if len(r) != 1 || r[0].duration != 50*time.Second {
		t.Fatalf("expected a report of the 10 s before and the 40 s after, got %+v", r)
	}
}

// A Time Threshold reached while a report is outstanding does not fire once duration
// measurement is turned off: its report would carry no duration.
func TestATimeThresholdDoesNotFireWhileDurationMeasurementIsOff(t *testing.T) {
	u := bothThresholdsURR()
	u.timeThreshold = 30 * time.Second
	s := measuredSession(u)

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch) // metering starts

	sent := s.dueReports(testEpoch)
	if len(sent) != 1 || sent[0].trigger != triggerVOLTH {
		t.Fatalf("expected a volume threshold report, got %+v", sent)
	}

	// Delivered only once 40 s are metered, past the threshold, and duration measurement
	// is off.
	setMeasurement(t, s, true, false, testEpoch.Add(40*time.Second))
	s.settleReports(sent, reportDelivered)

	if r := poll(s, someUplink(), testEpoch.Add(50*time.Second)); len(r) != 0 {
		t.Fatalf("a time threshold fired with duration measurement off: %+v", r)
	}
}

// A refused report's volume is kept while volume measurement is off, past a report
// without volume, for the next report that measures volume.
func TestARefusedVolumeIsKeptWhileVolumeMeasurementIsOff(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: time.Minute,
	})

	poll(s, downlink(0), testEpoch) // arms the period
	s.addUsage(downlinkPDRID, downlink(5*mb), testEpoch.Add(time.Second))

	refused := s.dueReports(testEpoch.Add(time.Minute))
	if len(refused) != 1 || refused[0].volume.total != 5*mb {
		t.Fatalf("expected a periodic report of 5 MB, got %+v", refused)
	}

	setMeasurement(t, s, false, true, testEpoch.Add(time.Minute))
	s.settleReports(refused, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Minute)); len(r) != 1 || r[0].hasVolume {
		t.Fatalf("expected a periodic report without volume, got %+v", r)
	}

	setMeasurement(t, s, true, false, testEpoch.Add(2*time.Minute))

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Minute)); len(r) != 1 || r[0].volume.total != 5*mb {
		t.Fatalf("expected the refused 5 MB in the next report that measures volume, got %+v", r)
	}
}

// A Volume Threshold a refusal restored does not fire while volume measurement is off;
// it fires, with the volume, once it is on again.
func TestARestoredVolumeThresholdWaitsForVolumeMeasurement(t *testing.T) {
	s := measuredSession(volumeThresholdURR(10 * mb))
	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch)

	refused := s.dueReports(testEpoch)
	if len(refused) != 1 || refused[0].trigger != triggerVOLTH {
		t.Fatalf("expected a volume threshold report, got %+v", refused)
	}

	setMeasurement(t, s, false, true, testEpoch)
	s.settleReports(refused, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a volume threshold fired with volume measurement off: %+v", r)
	}

	setMeasurement(t, s, true, false, testEpoch.Add(2*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Second)); len(r) != 1 ||
		r[0].trigger != triggerVOLTH || r[0].volume.total != 10*mb {
		t.Fatalf("expected the volume threshold report of 10 MB, got %+v", r)
	}
}

// A Create URR that replaces the URR and turns duration measurement off stops the
// metering as an Update URR does.
func TestAReplacingCreateURRStopsTheMetering(t *testing.T) {
	u := urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 110 * time.Second,
	}
	s := measuredSession(u)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	volumeOnly := u
	volumeOnly.measureVolume, volumeOnly.measureDuration = true, false
	s.CreateURR(volumeOnly, testEpoch.Add(10*time.Second))
	s.CreateURR(u, testEpoch.Add(100*time.Second))

	if r := poll(s, someUplink(), testEpoch.Add(100*time.Second)); len(r) != 0 {
		t.Fatalf("expected no report before the period ends, got %+v", r)
	}

	r := poll(s, volumeCounts{}, testEpoch.Add(110*time.Second))
	if len(r) != 1 || r[0].duration != 20*time.Second {
		t.Fatalf("expected a report of 20 s, got %+v", r)
	}
}

// Traffic seen before duration measurement is turned on does not start the metering: a
// URR that measured only volume meters time from the first traffic after it measures
// duration too.
func TestTrafficBeforeDurationMeasurementDoesNotStartTheMetering(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 110 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // traffic, and the period

	setMeasurement(t, s, true, true, testEpoch.Add(100*time.Second))
	poll(s, someUplink(), testEpoch.Add(100*time.Second))

	r := poll(s, volumeCounts{}, testEpoch.Add(110*time.Second))
	if len(r) != 1 || r[0].duration != 10*time.Second {
		t.Fatalf("expected a report of the 10 s since the first traffic metered, got %+v", r)
	}
}

// A Create URR that replaces the URR with another Measurement Period starts the new
// period afresh, as an Update URR does.
func TestAReplacingCreateURRWithAnotherPeriodStartsItAfresh(t *testing.T) {
	u := urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 10 * time.Second,
	}
	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	u.measurementPeriod = 100 * time.Second
	s.CreateURR(u, testEpoch.Add(5*time.Second))

	if r := poll(s, downlink(1), testEpoch.Add(10*time.Second)); len(r) != 0 {
		t.Fatalf("the replaced period fired: %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(110*time.Second)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report a new period after the replacement, got %+v", r)
	}
}

// A Create URR that replaces the URR and turns PERIO back on starts the period afresh.
func TestAReplacingCreateURRThatTurnsThePeriodOnStartsItAfresh(t *testing.T) {
	u := urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: time.Minute,
	}
	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	if err := s.UpdateURR(urr{urrID: testURRID}, urrIEs{reportingTriggers: true}, testEpoch); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	s.CreateURR(u, testEpoch.Add(100*time.Second))

	if r := poll(s, downlink(1), testEpoch.Add(100*time.Second)); len(r) != 0 {
		t.Fatalf("the period turned on again fired at once: %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(160*time.Second)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report a period after it was turned on again, got %+v", r)
	}
}

// A Create URR that repeats the URR's provisioning, as a control plane does that lost
// the response to a modification, keeps the running period.
func TestARepeatedCreateURRKeepsTheRunningPeriod(t *testing.T) {
	u := urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: time.Minute,
	}
	s := measuredSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	s.CreateURR(u, testEpoch.Add(30*time.Second))

	if r := poll(s, downlink(1), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected the periodic report when the running period ends, got %+v", r)
	}
}

// A period set while a report is outstanding starts at the next poll, not once the report
// has settled.
func TestAPeriodSetWhileAReportIsOutstandingStartsAtTheNextPoll(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 10 * time.Second,
	})

	poll(s, someUplink(), testEpoch) // arms the 10 s period

	sent := s.dueReports(testEpoch.Add(10 * time.Second))
	if len(sent) != 1 {
		t.Fatalf("expected the periodic report at 10 s, got %+v", sent)
	}

	if err := s.UpdateURR(urr{urrID: testURRID, measurementPeriod: 30 * time.Second},
		urrIEs{measurementPeriod: true}, testEpoch.Add(11*time.Second)); err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	s.dueReports(testEpoch.Add(12 * time.Second)) // the report is still being sent
	s.settleReports(sent, reportDelivered)

	var fired []time.Duration

	for _, at := range []time.Duration{13, 41, 42} {
		if r := poll(s, someUplink(), testEpoch.Add(at*time.Second)); len(r) > 0 {
			fired = append(fired, at)
		}
	}

	if len(fired) != 1 || fired[0] != 42 {
		t.Fatalf("periodic reports at %v s, expected one at 42 s: 30 s from the poll after the update", fired)
	}
}

// A poll a day late sends one report, and the next falls where the schedule puts it. The
// advance is one step, which this checks only by its result.
func TestAPollManyPeriodsLateKeepsTheSchedule(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureVolume:     true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: time.Second,
	})

	poll(s, someUplink(), testEpoch) // arms the 1 s period

	day := 24 * time.Hour

	var fired []time.Duration

	for _, at := range []time.Duration{day + 500*time.Millisecond, day + 900*time.Millisecond, day + time.Second} {
		if r := poll(s, someUplink(), testEpoch.Add(at)); len(r) > 0 {
			fired = append(fired, at)
		}
	}

	if want := []time.Duration{day + 500*time.Millisecond, day + time.Second}; !slices.Equal(fired, want) {
		t.Fatalf("periodic reports at %v, expected %v", fired, want)
	}
}

// A URR that loses its last PDR reports and is reset, but a report it had outstanding may
// still be refused, which gives its usage back. That usage does not fire a threshold while
// no PDR measures the URR: it would be reported as though it had just been measured. It
// goes in the next report.
func TestGivenBackUsageDoesNotFireAThresholdWhileNoPDRMeasuresTheURR(t *testing.T) {
	u := volumeThresholdURR(100)
	u.triggers.periodic = true
	u.measurementPeriod = 10 * time.Second

	s := measuredSession(u)
	before := slices.Clone(s.pdrs)

	poll(s, downlink(10), testEpoch) // the period starts

	// The period ends past the threshold: a report of 160 B is outstanding.
	s.addUsage(downlinkPDRID, downlink(150), testEpoch.Add(10*time.Second))
	sent := s.dueReports(testEpoch.Add(10 * time.Second))

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(11*time.Second))
	s.settleReports(sent, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(12*time.Second)); len(r) != 0 {
		t.Fatalf("expected no threshold report for usage no PDR measures, got %+v", r)
	}

	r := poll(s, volumeCounts{}, testEpoch.Add(21*time.Second))
	if len(r) != 1 || r[0].trigger != triggerPERIO || r[0].volume.total != 160 {
		t.Fatalf("expected the given-back 160 B in the next periodic report, got %+v", r)
	}
}

// The same for the Time Threshold: the time of a refused report, given back after the
// URR lost its last PDR, does not fire it.
func TestGivenBackTimeDoesNotFireAThresholdWhileNoPDRMeasuresTheURR(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true, timeThreshold: true},
		measurementPeriod: 100 * time.Second,
		timeThreshold:     50 * time.Second,
	})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	// The period ends past the threshold: a report of 100 s is outstanding.
	sent := s.dueReports(testEpoch.Add(100 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerPERIO|triggerTIMTH {
		t.Fatalf("expected a periodic and threshold report, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	// Nothing was metered since the report, so no termination report carries it again.
	if r := s.reportDissociated(before, testEpoch.Add(100*time.Second)); len(r) != 0 {
		t.Fatalf("expected no termination report, got %+v", r)
	}

	s.settleReports(sent, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(120*time.Second)); len(r) != 0 {
		t.Fatalf("expected no threshold report for time no PDR measures, got %+v", r)
	}

	r := poll(s, volumeCounts{}, testEpoch.Add(200*time.Second))
	if len(r) != 1 || r[0].trigger != triggerPERIO || r[0].duration != 100*time.Second {
		t.Fatalf("expected the given-back 100 s in the next periodic report, got %+v", r)
	}
}

// A URR that loses its last PDR counts its thresholds afresh once a PDR measures it again:
// its measurement was reset (clause 5.2.2.3.1 NOTE 5).
func TestADissociatedURRCountsItsThresholdsAfresh(t *testing.T) {
	s := measuredSession(urr{
		urrID:           testURRID,
		measureVolume:   true,
		measureDuration: true,
		triggers:        urrTriggers{volumeThreshold: true, timeThreshold: true},
		volumeThreshold: volumeLimit{total: 1000, hasTotal: true},
		timeThreshold:   100 * time.Second,
	})
	before := slices.Clone(s.pdrs)

	poll(s, downlink(600), testEpoch) // metering starts
	s.addUsage(downlinkPDRID, someUplink(), testEpoch.Add(60*time.Second))

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	r := s.reportDissociated(before, testEpoch.Add(60*time.Second))
	if len(r) != 1 || r[0].trigger != triggerTERMR || r[0].volume.total != 601 || r[0].duration != 60*time.Second {
		t.Fatalf("expected a termination report of 601 B and 60 s, got %+v", r)
	}

	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	if r := poll(s, downlink(600), testEpoch.Add(70*time.Second)); len(r) != 0 {
		t.Fatalf("expected no threshold report for 600 B and no time since the reset, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(130*time.Second)); len(r) != 0 {
		t.Fatalf("expected no threshold report for 60 s since the reset, got %+v", r)
	}
}

// A threshold report re-applies the thresholds with what they counted, which may include
// usage an earlier periodic report carried. If the URR loses its last PDR and is reset
// while that report is outstanding, and the report is then refused, only the usage the
// report carried goes back to the thresholds: what they counted before it was reset.
func TestARefusalAfterADissociationGivesTheThresholdOnlyTheReportsVolume(t *testing.T) {
	u := volumeThresholdURR(1000)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)
	before := slices.Clone(s.pdrs)

	poll(s, downlink(0), testEpoch) // arms the period

	if r := poll(s, downlink(600), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(400), testEpoch.Add(61*time.Second))

	sent := s.dueReports(testEpoch.Add(61 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLTH || sent[0].volume.total != 400 {
		t.Fatalf("expected a volume threshold report of the 400 B since the periodic one, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(62*time.Second))
	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	if r := poll(s, downlink(599), testEpoch.Add(63*time.Second)); len(r) != 0 {
		t.Fatalf("expected no threshold report for the 400 B given back and 599 B since, got %+v", r)
	}

	r := poll(s, downlink(1), testEpoch.Add(64*time.Second))
	if len(r) != 1 || r[0].trigger != triggerVOLTH || r[0].volume.total != 1000 {
		t.Fatalf("expected a threshold report of the 400 B given back and 600 B since, got %+v", r)
	}
}

// The same for the Time Threshold.
func TestARefusalAfterADissociationGivesTheThresholdOnlyTheReportsTime(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true, timeThreshold: true},
		measurementPeriod: time.Minute,
		timeThreshold:     100 * time.Second,
	})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	if r := poll(s, volumeCounts{}, testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	sent := s.dueReports(testEpoch.Add(100 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerTIMTH || sent[0].duration != 40*time.Second {
		t.Fatalf("expected a time threshold report of the 40 s since the periodic one, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(100*time.Second))
	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	timeThresholdReport := func(r *usageReport) bool { return r.trigger&triggerTIMTH != 0 }

	// Metering starts again with the traffic at 110 s.
	if r := poll(s, someUplink(), testEpoch.Add(110*time.Second)); slices.ContainsFunc(r, timeThresholdReport) {
		t.Fatalf("expected no threshold report for the 40 s given back, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(169*time.Second)); slices.ContainsFunc(r, timeThresholdReport) {
		t.Fatalf("expected no threshold report for the 40 s given back and 59 s since, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(170*time.Second)); !slices.ContainsFunc(r, timeThresholdReport) {
		t.Fatalf("expected a threshold report for the 40 s given back and 60 s since, got %+v", r)
	}
}

// A refused report of another trigger gives its usage back to the thresholds too. Without
// a reset that usage never left them; the reset discards it, and the refusal gives it back
// with the report's usage, so the next threshold report carries all the control plane has
// not counted.
func TestARefusedPeriodicReportAfterADissociationCountsTowardTheThresholds(t *testing.T) {
	u := volumeThresholdURR(1000)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)
	before := slices.Clone(s.pdrs)

	poll(s, downlink(0), testEpoch) // arms the period

	s.addUsage(downlinkPDRID, downlink(600), testEpoch.Add(time.Minute))

	sent := s.dueReports(testEpoch.Add(time.Minute))
	if len(sent) != 1 || sent[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(61*time.Second))
	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	if r := poll(s, downlink(399), testEpoch.Add(62*time.Second)); len(r) != 0 {
		t.Fatalf("expected no report for the 600 B given back and 399 B since, got %+v", r)
	}

	r := poll(s, downlink(1), testEpoch.Add(63*time.Second))
	if len(r) != 1 || r[0].trigger != triggerVOLTH || r[0].volume.total != 1000 {
		t.Fatalf("expected a threshold report of the 600 B given back and 400 B since, got %+v", r)
	}
}

// A report refused after a reset, which no termination report carried again, gives back
// only the whole seconds it reported. The part of a second it left was dropped with the
// reset, as it is when the report is delivered, and the part it took from the report
// before it is in those whole seconds.
func TestARefusalAfterADissociationGivesBackOnlyTheReportsWholeSeconds(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true},
		measurementPeriod: 1300 * time.Millisecond,
	})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	if r := poll(s, volumeCounts{}, testEpoch.Add(1300*time.Millisecond)); len(r) != 1 || r[0].duration != time.Second {
		t.Fatalf("expected a periodic report of 1 s, leaving 0.3 s, got %+v", r)
	}

	sent := s.dueReports(testEpoch.Add(2600 * time.Millisecond))
	if len(sent) != 1 || sent[0].duration != time.Second {
		t.Fatalf("expected a periodic report of 1 s, taking 0.3 s and leaving 0.6 s, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	if r := s.reportDissociated(before, testEpoch.Add(2900*time.Millisecond)); len(r) != 0 {
		t.Fatalf("expected no termination report for the 0.6 s left and 0.3 s since, got %+v", r)
	}

	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	// Metering starts again with the traffic at 3 s.
	s.addUsage(downlinkPDRID, someUplink(), testEpoch.Add(3*time.Second))
	s.addUsage(downlinkPDRID, someUplink(), testEpoch.Add(3500*time.Millisecond))

	u := s.findURR(testURRID)
	if got := u.report(triggerPERIO, testEpoch.Add(3500*time.Millisecond)); got.duration != time.Second {
		t.Fatalf("expected the 1 s given back to be reported with the 0.5 s since, as 1 s, got %v", got.duration)
	}

	if u.usage.durationCarry != 500*time.Millisecond {
		t.Fatalf("expected 0.5 s carried, got %v", u.usage.durationCarry)
	}
}

// The same whole seconds, and no more, go back to the Time Threshold.
func TestARefusalAfterADissociationGivesTheTimeThresholdOnlyWholeSeconds(t *testing.T) {
	s := measuredSession(urr{
		urrID:             testURRID,
		measureDuration:   true,
		triggers:          urrTriggers{periodic: true, timeThreshold: true},
		measurementPeriod: 1500 * time.Millisecond,
		timeThreshold:     3 * time.Second,
	})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts, and the period

	sent := s.dueReports(testEpoch.Add(1500 * time.Millisecond))
	if len(sent) != 1 || sent[0].duration != time.Second {
		t.Fatalf("expected a periodic report of 1 s, leaving 0.5 s, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	if r := s.reportDissociated(before, testEpoch.Add(1600*time.Millisecond)); len(r) != 0 {
		t.Fatalf("expected no termination report for the 0.5 s left and 0.1 s since, got %+v", r)
	}

	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	timeThresholdReport := func(r *usageReport) bool { return r.trigger&triggerTIMTH != 0 }

	// Metering starts again with the traffic at 3 s.
	s.addUsage(downlinkPDRID, someUplink(), testEpoch.Add(3*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(4900*time.Millisecond)); slices.ContainsFunc(r, timeThresholdReport) {
		t.Fatalf("expected no threshold report for the 1 s given back and 1.9 s since, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(5*time.Second)); !slices.ContainsFunc(r, timeThresholdReport) {
		t.Fatalf("expected a threshold report for the 1 s given back and 2 s since, got %+v", r)
	}
}

// A report the URR had outstanding when it lost its last PDR goes out again in the
// modification response, with its UR-SEQN, ahead of the termination report. The control
// plane may have counted that copy, so a later refusal of the Session Report Request
// that carried it gives nothing back: the usage would be billed again under a new
// UR-SEQN. A report the URR generates later is settled as before.
func TestARefusalOfAReportReplayedInAResponseGivesNothingBack(t *testing.T) {
	s := measuredSession(volumeThresholdURR(100))
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(150), testEpoch)
	sent := s.dueReports(testEpoch)
	s.addUsage(downlinkPDRID, downlink(30), testEpoch.Add(time.Second))

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	r := s.reportDissociated(before, testEpoch.Add(time.Second))
	if len(r) != 2 || r[0].volume.total != 150 || r[1].volume.total != 30 {
		t.Fatalf("expected the outstanding 150 B, then a termination report of 30 B, got %+v", r)
	}

	// Unanswered, it is still sent again with its UR-SEQN until the control plane answers.
	s.settleReports(sent, reportUndelivered)

	if u := s.findURR(testURRID); u.usage.outstanding != sent[0] {
		t.Fatal("an unanswered report the response carried was dropped")
	}

	s.settleReports(sent, reportRejected)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	if r := poll(s, downlink(99), testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("expected no report: the refused 150 B were in the response, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(1), testEpoch.Add(3*time.Second))

	sent = s.dueReports(testEpoch.Add(3 * time.Second))
	if len(sent) != 1 || sent[0].seq != 2 || sent[0].volume.total != 100 {
		t.Fatalf("expected a threshold report of the 100 B since, with UR-SEQN 2, got %+v", sent)
	}

	s.settleReports(sent, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(4*time.Second)); len(r) != 1 || r[0].volume.total != 100 {
		t.Fatalf("expected the refused 100 B, which no response carried, in the next report, got %+v", r)
	}
}

// Once the report outstanding at the reset is settled, a later refused threshold report
// restores what the thresholds counted, as it does for a URR never reset: the mark the
// reset left ends with the report it was made for.
func TestARefusalAfterTheResetReportIsSettledRestoresTheThresholds(t *testing.T) {
	u := volumeThresholdURR(1000)
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := measuredSession(u)
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(1000), testEpoch)
	sent := s.dueReports(testEpoch)

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch)
	s.settleReports(sent, reportDelivered)
	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	poll(s, downlink(0), testEpoch.Add(time.Second)) // arms the period

	if r := poll(s, downlink(600), testEpoch.Add(61*time.Second)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(400), testEpoch.Add(62*time.Second))

	sent = s.dueReports(testEpoch.Add(62 * time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLTH {
		t.Fatalf("expected a volume threshold report, got %+v", sent)
	}

	s.settleReports(sent, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(63*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected the volume threshold report to be due again, got %+v", r)
	}
}

// A URR that loses its last PDR with a report outstanding sends it again ahead of its
// termination report of what it measured since, so the control plane sees them in order.
// The outstanding report stays to be settled.
func TestADissociatedURRSendsItsOutstandingReportFirst(t *testing.T) {
	s := measuredSession(volumeThresholdURR(100))
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(150), testEpoch)
	sent := s.dueReports(testEpoch)
	s.addUsage(downlinkPDRID, downlink(30), testEpoch.Add(time.Second))

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	r := s.reportDissociated(before, testEpoch.Add(time.Second))
	if len(r) != 2 || r[0].seq != 0 || r[0].volume.total != 150 ||
		r[1].seq != 1 || r[1].trigger != triggerTERMR || r[1].volume.total != 30 {
		t.Fatalf("expected the outstanding 150 B with UR-SEQN 0, then a termination report of "+
			"the 30 B since with UR-SEQN 1, got %+v", r)
	}

	if u := s.findURR(testURRID); u.usage.outstanding != sent[0] {
		t.Fatal("the outstanding report was dropped")
	}
}

// A URR measuring only duration reports the time it metered when it loses its last PDR.
func TestADissociatedURRReportsItsDuration(t *testing.T) {
	s := measuredSession(urr{urrID: testURRID, measureDuration: true})
	before := slices.Clone(s.pdrs)

	poll(s, someUplink(), testEpoch) // metering starts

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	r := s.reportDissociated(before, testEpoch.Add(60*time.Second))
	if len(r) != 1 || r[0].trigger != triggerTERMR || r[0].duration != 60*time.Second {
		t.Fatalf("expected a termination report of 60 s, got %+v", r)
	}
}

// A URR that loses its last PDR is reset even when it has nothing to report: the volume it
// kept while volume measurement is off, the part of a second it metered, and its counts
// toward its thresholds are discarded, and its next report starts now.
func TestADissociatedURRIsResetWhenItDoesNotReport(t *testing.T) {
	s := measuredSession(urr{urrID: testURRID, measureDuration: true})
	before := slices.Clone(s.pdrs)

	s.urrs[0].usage = urrUsage{
		since:               testEpoch,
		unreported:          downlink(500), // kept from when volume was measured
		toThreshold:         downlink(500),
		metered:             500 * time.Millisecond,
		toTimeThreshold:     500 * time.Millisecond,
		durationCarry:       300 * time.Millisecond,
		volumeThresholdSent: true,
		timeThresholdSent:   true,
		nextSeq:             3,
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	now := testEpoch.Add(time.Minute)
	if r := s.reportDissociated(before, now); len(r) != 0 {
		t.Fatalf("expected no report of less than a second, got %+v", r)
	}

	want := urrUsage{since: now, nextSeq: 3}
	if got := s.urrs[0].usage; got != want {
		t.Fatalf("the URR was left with %+v, expected %+v", got, want)
	}
}
