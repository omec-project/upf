// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import "time"

// urrUsage is what this UPF has measured for a URR, and when.
type urrUsage struct {
	// since is when the measurement the next report covers began: the URR's creation or
	// its last report. It is the report's Start Time.
	since time.Time

	// unreported is the volume measured since the last report. toThreshold is the volume
	// measured since the Volume Threshold was last applied: a threshold report re-applies
	// every threshold, a report for another trigger none (TS 29.244 clause 5.2.2.3.1).
	unreported  volumeCounts
	toThreshold volumeCounts

	// meteredTo is the instant up to which time has been metered into metered and
	// toTimeThreshold. Time is metered from the first traffic this UPF sees for the URR
	// (clause 5.2.2.2.1), to within a poll, and only while the URR measures duration:
	// meteredTo is zero before that traffic, and from when duration measurement is
	// turned off until the first traffic after it is turned on again.
	meteredTo time.Time
	// metered is the time metered since the last report. toTimeThreshold is the time
	// metered since the Time Threshold was last applied.
	metered         time.Duration
	toTimeThreshold time.Duration
	// durationCarry is the part of a second the last report's Duration Measurement, which
	// counts whole seconds, could not carry.
	durationCarry time.Duration

	// volumeThresholdSent and timeThresholdSent say the control plane sent that threshold
	// since the last report. If that report is refused, the threshold is counted from the
	// report before it, the last the control plane counted, rather than restored to what
	// the refused report re-applied.
	volumeThresholdSent bool
	timeThresholdSent   bool

	// nextPeriod is when the measurement period next ends. Zero until the URR has a
	// period.
	nextPeriod time.Time

	// nextSeq is the UR-SEQN of the next report: 0 for the first, then one more for each
	// (clause 5.2.2.3.1).
	nextSeq uint32

	// outstanding is a report that has not been delivered, and inFlight says whether it is
	// being sent. Until it is delivered the URR starts no other report: its usage
	// accumulates for the next one. An outstanding report that did not reach the control
	// plane is sent again as it was, with its UR-SEQN, so that a control plane that did
	// receive it, and lost only its answer, counts it once.
	outstanding *usageReport
	inFlight    bool
}

// CreateURR adds a URR to the session. A Create URR naming a URR the session already
// holds replaces its provisioning and keeps what was measured for it, so that a control
// plane that repeats a modification whose response it lost neither loses the URR's usage
// nor has it reported twice. A changed period starts afresh, as Update URR has it.
func (s *PFCPSession) CreateURR(u urr, now time.Time) {
	for i := range s.urrs {
		if held := s.urrs[i]; held.urrID == u.urrID {
			u.usage = held.usage
			u.usage.setMeteringDuration(held.measureDuration, u.measureDuration, now)

			if u.measurementPeriod != held.measurementPeriod || (u.triggers.periodic && !held.triggers.periodic) {
				u.usage.nextPeriod = time.Time{}
			}

			s.urrs[i] = u

			return
		}
	}

	u.usage = urrUsage{since: now}
	s.urrs = append(s.urrs, u)
}

// UpdateURR changes the IEs an Update URR carried and keeps the rest (clause 7.5.4.4).
func (s *PFCPSession) UpdateURR(u urr, has urrIEs, now time.Time) error {
	held := s.findURR(u.urrID)
	if held == nil {
		return ErrNotFoundWithParam("URR", "id", u.urrID)
	}

	if has.measurementMethod {
		held.usage.setMeteringDuration(held.measureDuration, u.measureDuration, now)
		held.measureVolume = u.measureVolume
		held.measureDuration = u.measureDuration
	}

	if has.reportingTriggers {
		// A period turned on again starts afresh, not from where it was turned off.
		if u.triggers.periodic && !held.triggers.periodic {
			held.usage.nextPeriod = time.Time{}
		}

		held.triggers = u.triggers
	}

	if has.measurementPeriod {
		held.measurementPeriod = u.measurementPeriod
		// The new period runs from the next poll.
		held.usage.nextPeriod = time.Time{}
	}

	// A threshold is new whatever its value, and is counted from the URR's last report:
	// the traffic and time since then are deducted from it (clause 5.2.2.3.1 NOTE 1).
	if has.volumeThreshold {
		held.volumeThreshold = u.volumeThreshold
		held.usage.toThreshold = held.usage.unreported
		held.usage.volumeThresholdSent = true
	}

	if has.timeThreshold {
		held.timeThreshold = u.timeThreshold
		held.usage.toTimeThreshold = held.usage.metered
		held.usage.timeThresholdSent = true
	}

	return nil
}

// RemoveURR removes the URR and returns it as it was.
func (s *PFCPSession) RemoveURR(id uint32) (urr, error) {
	for i, u := range s.urrs {
		if u.urrID == id {
			s.urrs = append(s.urrs[:i], s.urrs[i+1:]...)
			return u, nil
		}
	}

	return urr{}, ErrNotFoundWithParam("URR", "id", id)
}

func (s *PFCPSession) findURR(id uint32) *urr {
	for i := range s.urrs {
		if s.urrs[i].urrID == id {
			return &s.urrs[i]
		}
	}

	return nil
}

// addUsage credits the volume the datapath counted for a PDR to the URRs that measure it.
func (s *PFCPSession) addUsage(pdrID uint32, v volumeCounts, at time.Time) {
	p, ok := s.findPDR(pdrID)
	if !ok {
		return
	}

	for _, id := range p.urrIDList {
		if u := s.findURR(id); u != nil {
			u.addVolume(v, at)
		}
	}
}

func (u *urr) addVolume(v volumeCounts, at time.Time) {
	if v.isZero() {
		return
	}

	if u.measureDuration && u.usage.meteredTo.IsZero() {
		u.usage.meteredTo = at
	}

	if u.measureVolume {
		u.usage.unreported.add(v)
		u.usage.toThreshold.add(v)
	}
}

// meter meters the time up to now, if time is being metered. Polls and modifications take
// their time under the association's session lock, so now is never before meteredTo.
func (us *urrUsage) meter(now time.Time) {
	if us.meteredTo.IsZero() {
		return
	}

	d := now.Sub(us.meteredTo)
	us.metered += d
	us.toTimeThreshold += d
	us.meteredTo = now
}

// setMeteringDuration follows a change of the URR's measurement method. Time metered
// before duration measurement is turned off is kept for the next report that measures
// duration; the time it is off is not metered.
func (us *urrUsage) setMeteringDuration(was, is bool, now time.Time) {
	if was && !is {
		us.stopMetering(now)
	}
}

// stopMetering meters the time up to now and stops. Metering starts again at the next
// traffic credited to the URR while it measures duration.
func (us *urrUsage) stopMetering(now time.Time) {
	us.meter(now)
	us.meteredTo = time.Time{}
}

// measures says whether a PDR of the session measures URR id.
func (s *PFCPSession) measures(id uint32) bool {
	return measuredBy(s.pdrs, id)
}

// measuredBy says whether one of pdrs measures URR id.
func measuredBy(pdrs []pdr, id uint32) bool {
	for _, p := range pdrs {
		for _, u := range p.urrIDList {
			if u == id {
				return true
			}
		}
	}

	return false
}

// reportDissociated returns the termination reports of the URRs a modification left
// without a PDR: each URR one of the PDRs the session had before measured, and none of
// its PDRs measures now. Such a URR is kept, and its measurement is stopped and reset; it
// reports what it measured since its last report if that is not null (TS 29.244 clauses
// 5.2.2.3.1 and 8.2.41). A report it has outstanding goes ahead of that one, with its
// UR-SEQN, so that the control plane sees them in order; it stays outstanding, to be
// settled as before.
func (s *PFCPSession) reportDissociated(before []pdr, now time.Time) []usageReport {
	var reports []usageReport

	for i := range s.urrs {
		u := &s.urrs[i]
		if !measuredBy(before, u.urrID) || s.measures(u.urrID) {
			continue
		}

		u.usage.stopMetering(now)

		if u.hasMeasurement() {
			if r := u.usage.outstanding; r != nil {
				reports = append(reports, *r)
			}

			reports = append(reports, u.report(triggerTERMR, now))
		}

		u.usage.reset(now)
	}

	return reports
}

// hasMeasurement says whether the URR's next report would carry a measurement that is
// not null: volume, or at least a whole second, of a measurement that is on.
func (u *urr) hasMeasurement() bool {
	us := &u.usage

	return u.measureVolume && !us.unreported.isZero() ||
		u.measureDuration && us.metered+us.durationCarry >= time.Second
}

// reset discards what the URR measured. Clause 5.2.2.3.1 has the UP function "reset any
// measurement for the URR" when its last PDR is removed; this includes the counts toward
// its thresholds and usage kept while a measurement is off. Its UR-SEQN, period and
// outstanding report run on.
func (us *urrUsage) reset(now time.Time) {
	us.since = now
	us.unreported, us.toThreshold = volumeCounts{}, volumeCounts{}
	us.metered, us.toTimeThreshold, us.durationCarry = 0, 0, 0
	us.volumeThresholdSent, us.timeThresholdSent = false, false
}

// dueReports returns the reports the session's URRs are to send now: each outstanding
// report not being sent, and a new report from each URR whose triggers have fired. It
// marks every report it returns as being sent.
func (s *PFCPSession) dueReports(now time.Time) []*usageReport {
	var reports []*usageReport

	for i := range s.urrs {
		u := &s.urrs[i]

		if r := u.usage.outstanding; r != nil {
			u.startPeriod(now)

			if !u.usage.inFlight {
				u.usage.inFlight = true
				reports = append(reports, r)
			}

			continue
		}

		t := u.due(now)

		// A URR no PDR measures is credited no traffic. What it holds -- usage a refused
		// report gave back after its last PDR left -- is kept for its next report, and does
		// not fire a threshold: that would report the usage as though it had just been
		// measured.
		if t&(triggerVOLTH|triggerTIMTH) != 0 && !s.measures(u.urrID) {
			t &^= triggerVOLTH | triggerTIMTH
		}

		if t != 0 {
			r := u.report(t, now)
			u.usage.outstanding, u.usage.inFlight = &r, true
			reports = append(reports, &r)
		}
	}

	return reports
}

// due returns the triggers that have fired for the URR by now.
func (u *urr) due(now time.Time) reportTrigger {
	var t reportTrigger

	us := &u.usage
	us.meter(now)

	// A threshold fires only while its measurement is on, so that its report carries it.
	if u.triggers.volumeThreshold && u.measureVolume && u.volumeThreshold.reachedBy(us.toThreshold) {
		t |= triggerVOLTH
	}

	if u.triggers.timeThreshold && u.measureDuration && u.timeThreshold > 0 &&
		us.toTimeThreshold >= u.timeThreshold {
		t |= triggerTIMTH
	}

	u.startPeriod(now)

	if u.triggers.periodic && u.measurementPeriod > 0 && !now.Before(us.nextPeriod) {
		t |= triggerPERIO

		// One report for the periods a stalled poll let pass, not one for each, and the
		// next period where the schedule puts it.
		missed := now.Sub(us.nextPeriod) / u.measurementPeriod
		us.nextPeriod = us.nextPeriod.Add((missed + 1) * u.measurementPeriod)
	}

	return t
}

// startPeriod starts the URR's periodic reporting, if it is on and not yet running: at the
// first poll after the provisioning that set or changed its period, whether or not a
// report is outstanding then.
func (u *urr) startPeriod(now time.Time) {
	if u.triggers.periodic && u.measurementPeriod > 0 && u.usage.nextPeriod.IsZero() {
		u.usage.nextPeriod = now.Add(u.measurementPeriod)
	}
}

// report generates the URR's next usage report and resets the measurement it reports
// (clause 5.2.2.3.1).
//
// A measurement that is off is not reported, and what it measured while it was on is kept
// for the next report once it is on again.
func (u *urr) report(t reportTrigger, now time.Time) usageReport {
	us := &u.usage
	us.meter(now)

	r := usageReport{
		urrID:         u.urrID,
		seq:           us.nextSeq,
		trigger:       t,
		start:         us.since,
		end:           now,
		durationCarry: us.durationCarry,
	}

	us.nextSeq++

	if u.measureVolume {
		r.hasVolume, r.volume = true, us.unreported
		us.unreported = volumeCounts{}
	}

	if u.measureDuration {
		d := us.metered + us.durationCarry
		r.hasDuration, r.metered, r.duration = true, us.metered, d.Truncate(time.Second)
		us.metered, us.durationCarry = 0, d-r.duration
	}

	us.since = now
	us.volumeThresholdSent, us.timeThresholdSent = false, false

	// A threshold report re-applies all of the URR's thresholds; a report for another
	// trigger leaves them counting on (clause 5.2.2.3.1).
	if t&(triggerVOLTH|triggerTIMTH) != 0 {
		r.reapplied = &thresholdState{us.toThreshold, us.toTimeThreshold}
		us.toThreshold, us.toTimeThreshold = volumeCounts{}, 0
	}

	return r
}

// reportOutcome is what became of a report that was sent.
type reportOutcome int

const (
	// reportDelivered: the control plane accepted it.
	reportDelivered reportOutcome = iota
	// reportRejected: the control plane answered and refused it, so it counted none of it.
	reportRejected
	// reportUndelivered: no answer, or the report could not be sent. The control plane
	// may have counted it.
	reportUndelivered
)

// settleReports records what became of reports the session's URRs sent. A report that is
// no longer the URR's outstanding one -- the URR was removed, or replaced by a new URR of
// the same ID, or the report settled already -- is left alone. Reports are matched by
// identity: a URR re-created under the same ID numbers its reports from 0 again.
func (s *PFCPSession) settleReports(sent []*usageReport, outcome reportOutcome) {
	for _, r := range sent {
		u := s.findURR(r.urrID)
		if u == nil || u.usage.outstanding != r {
			continue
		}

		us := &u.usage

		switch outcome {
		case reportDelivered:
			us.outstanding, us.inFlight = nil, false
		case reportRejected:
			// Refused, so not counted: the next report carries its usage instead, from its
			// Start Time, with the carry it took. A threshold report had re-applied the
			// thresholds; they count on from where they were. A threshold sent since the
			// report is counted from the report before it (clause 5.2.2.3.1 NOTE 1).
			us.unreported.add(r.volume)
			us.metered += r.metered
			us.since = r.start
			us.durationCarry = r.durationCarry

			p := r.reapplied

			switch {
			case us.volumeThresholdSent:
				us.toThreshold = us.unreported
			case p != nil:
				us.toThreshold.add(p.toThreshold)
			}

			switch {
			case us.timeThresholdSent:
				us.toTimeThreshold = us.metered
			case p != nil:
				us.toTimeThreshold += p.toTimeThreshold
			}

			us.outstanding, us.inFlight = nil, false
		case reportUndelivered:
			us.inFlight = false
		}
	}
}

// finalReports are the reports of a URR leaving its session: its outstanding report, if
// it has one, and a termination report of what it measured since (TS 29.244 clauses
// 5.2.2.3.1 and 8.2.41). The outstanding one may have reached the control plane already;
// it is sent again with its UR-SEQN, so it is counted once.
func (u *urr) finalReports(now time.Time) []usageReport {
	var reports []usageReport

	if r := u.usage.outstanding; r != nil {
		reports = append(reports, *r)
	}

	return append(reports, u.report(triggerTERMR, now))
}
