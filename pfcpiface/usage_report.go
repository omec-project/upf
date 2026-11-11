// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"time"

	"github.com/wmnsk/go-pfcp/ie"
)

// reportTrigger is a Usage Report Trigger (TS 29.244 clause 8.2.41): octet 5 in the low
// byte, octet 6 in the high one.
type reportTrigger uint16

const (
	triggerPERIO reportTrigger = 1 << 0 // periodic, octet 5 bit 1
	triggerVOLTH reportTrigger = 1 << 1 // volume threshold, octet 5 bit 2
	triggerTIMTH reportTrigger = 1 << 2 // time threshold, octet 5 bit 3
)

// Volume Measurement flags, clause 8.2.44: the volume counters it carries.
const volumeMeasurementAllCounters = 0x07 // TOVOL | ULVOL | DLVOL

// usageReport is one Usage Report of a URR (clause 7.5.8.3).
type usageReport struct {
	urrID   uint32
	seq     uint32 // UR-SEQN
	trigger reportTrigger

	// start and end are the Start Time and End Time of the measurement the report covers.
	start, end time.Time

	hasVolume bool
	volume    volumeCounts

	hasDuration bool
	duration    time.Duration

	// metered is the time metered that this report's duration counts, and durationCarry
	// the carry the URR had before this report took it, so that a refusal can return
	// them.
	metered       time.Duration
	durationCarry time.Duration

	// reapplied is what the thresholds had counted when this threshold report re-applied
	// them, so that a refusal can restore it. Nil for a report of another trigger.
	reapplied *thresholdState
}

// thresholdState is what a URR's thresholds have counted: the volume and the time.
type thresholdState struct {
	toThreshold     volumeCounts
	toTimeThreshold time.Duration
}

// ies are the IEs of the report. A volume-measured URR reports all three counters, zero
// or not (clause 5.2.2.3.1), and every report carries its Start and End Time: they are
// left out only of start- and stop-of-traffic reports, which this UPF does not send
// (Table 7.5.8.3-1).
func (r usageReport) ies() []*ie.IE {
	ies := []*ie.IE{
		ie.NewURRID(r.urrID),
		ie.NewURSEQN(r.seq),
		ie.NewUsageReportTrigger(uint8(r.trigger), uint8(r.trigger>>8)),
		ie.NewStartTime(r.start),
		ie.NewEndTime(r.end),
	}

	if r.hasVolume {
		ies = append(ies, ie.NewVolumeMeasurement(volumeMeasurementAllCounters,
			r.volume.total, r.volume.uplink, r.volume.downlink, 0, 0, 0))
	}

	if r.hasDuration {
		ies = append(ies, ie.NewDurationMeasurement(r.duration))
	}

	return ies
}

func (r usageReport) inSessionReportRequest() *ie.IE {
	return ie.NewUsageReportWithinSessionReportRequest(r.ies()...)
}
