// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"fmt"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
)

// volumeCounts is a volume in bytes for each counter of a Volume Measurement: total,
// uplink and downlink (TS 29.244 clause 8.2.44).
type volumeCounts struct {
	total, uplink, downlink uint64
}

func (c *volumeCounts) add(d volumeCounts) {
	c.total += d.total
	c.uplink += d.uplink
	c.downlink += d.downlink
}

func (c volumeCounts) isZero() bool {
	return c == volumeCounts{}
}

// volumeLimit is a Volume Threshold or a Volume Quota (clauses 8.2.13 and 8.2.50): a
// limit for each counter the control plane set.
type volumeLimit struct {
	total, uplink, downlink          uint64
	hasTotal, hasUplink, hasDownlink bool
}

// reachedBy says whether a count has reached any of the limits that are set. A limit of
// zero sets no limit.
func (l volumeLimit) reachedBy(c volumeCounts) bool {
	return l.hasTotal && l.total > 0 && c.total >= l.total ||
		l.hasUplink && l.uplink > 0 && c.uplink >= l.uplink ||
		l.hasDownlink && l.downlink > 0 && c.downlink >= l.downlink
}

// urrTriggers are the reporting triggers (clause 8.2.19) this UPF acts on. A URR may set
// others; they are accepted and do not cause a report.
type urrTriggers struct {
	periodic        bool // PERIO
	volumeThreshold bool // VOLTH
	timeThreshold   bool // TIMTH
	volumeQuota     bool // VOLQU
}

// urr is one Usage Reporting Rule: what the control plane provisioned, and what this UPF
// has measured for it.
type urr struct {
	urrID uint32

	measureVolume   bool
	measureDuration bool
	triggers        urrTriggers

	measurementPeriod time.Duration
	volumeThreshold   volumeLimit
	timeThreshold     time.Duration
	volumeQuota       volumeLimit

	usage urrUsage
}

func (u urr) String() string {
	return fmt.Sprintf("URR(id=%d, volume=%v, duration=%v, triggers=%+v, period=%v, "+
		"volumeThreshold=%+v, timeThreshold=%v, volumeQuota=%+v)",
		u.urrID, u.measureVolume, u.measureDuration, u.triggers, u.measurementPeriod,
		u.volumeThreshold, u.timeThreshold, u.volumeQuota)
}

// urrIEs records which of a URR's optional IEs a Create or Update URR carried. An Update
// URR changes only what it carries (clause 7.5.4.4), and a Volume Quota IE in one is a
// new quota whatever its value (clause 5.2.2.3.1).
type urrIEs struct {
	measurementMethod bool
	reportingTriggers bool
	measurementPeriod bool
	volumeThreshold   bool
	timeThreshold     bool
	volumeQuota       bool
}

var (
	errURRMeasurementMethodMissing = errors.New("create URR without the mandatory Measurement Method IE")
	errURRReportingTriggersMissing = errors.New("create URR without the mandatory Reporting Triggers IE")
	errURRNoFlagSet                = errors.New("no flag set, where at least one shall be")
)

// The flags of Measurement Method (clause 8.2.40), and of Volume Threshold and Volume
// Quota (clauses 8.2.13 and 8.2.50), of which at least one shall be set.
const (
	measurementMethodFlags = 0x07 // DURAT, VOLUM, EVENT
	volumeFlags            = 0x07 // TOVOL, ULVOL, DLVOL
)

// parseURR reads a Create URR or Update URR IE. A Create URR must carry a Measurement
// Method and Reporting Triggers (Table 7.5.2.4-1); an Update URR may leave out any IE but
// the URR ID.
func parseURR(urrIE *ie.IE, create bool) (urr, urrIEs, error) {
	var (
		u   urr
		has urrIEs
	)

	id, err := urrIE.URRID()
	if err != nil {
		return u, has, fmt.Errorf("URR ID: %w", err)
	}

	u.urrID = id

	var children []*ie.IE

	if create {
		children, err = urrIE.CreateURR()
	} else {
		children, err = urrIE.UpdateURR()
	}

	if err != nil {
		return u, has, fmt.Errorf("URR %d: %w", id, err)
	}

	for _, x := range children {
		switch x.Type {
		// The IEs' flag accessors read an IE that does not parse as no flag set, which
		// would turn measuring or reporting off; such an IE refuses the URR instead.
		case ie.MeasurementMethod:
			var m uint8
			if m, err = x.MeasurementMethod(); err != nil {
				return u, has, fmt.Errorf("URR %d Measurement Method: %w", id, err)
			}

			if m&measurementMethodFlags == 0 {
				return u, has, fmt.Errorf("URR %d Measurement Method: %w", id, errURRNoFlagSet)
			}

			has.measurementMethod = true
			u.measureVolume = x.HasVOLUM()
			u.measureDuration = x.HasDURAT()

		case ie.ReportingTriggers:
			var octets []byte
			if octets, err = x.ReportingTriggers(); err != nil {
				return u, has, fmt.Errorf("URR %d Reporting Triggers: %w", id, err)
			}

			// Clause 8.2.19: at least one trigger is set. ReportingTriggers ensures two
			// octets; a third, if present, has two triggers and spare bits.
			if octets[0] == 0 && octets[1] == 0 && (len(octets) < 3 || octets[2]&0x03 == 0) {
				return u, has, fmt.Errorf("URR %d Reporting Triggers: %w", id, errURRNoFlagSet)
			}

			has.reportingTriggers = true
			u.triggers = urrTriggers{
				periodic:        x.HasPERIO(),
				volumeThreshold: x.HasVOLTH(),
				timeThreshold:   x.HasTIMTH(),
				volumeQuota:     x.HasVOLQU(),
			}

		case ie.MeasurementPeriod:
			if u.measurementPeriod, err = x.MeasurementPeriod(); err != nil {
				return u, has, fmt.Errorf("URR %d Measurement Period: %w", id, err)
			}

			has.measurementPeriod = true

		case ie.VolumeThreshold:
			var f *ie.VolumeThresholdFields
			if f, err = x.VolumeThreshold(); err != nil {
				return u, has, fmt.Errorf("URR %d Volume Threshold: %w", id, err)
			}

			if f.Flags&volumeFlags == 0 {
				return u, has, fmt.Errorf("URR %d Volume Threshold: %w", id, errURRNoFlagSet)
			}

			has.volumeThreshold = true
			u.volumeThreshold = volumeLimit{
				total: f.TotalVolume, uplink: f.UplinkVolume, downlink: f.DownlinkVolume,
				hasTotal: f.HasTOVOL(), hasUplink: f.HasULVOL(), hasDownlink: f.HasDLVOL(),
			}

		case ie.TimeThreshold:
			if u.timeThreshold, err = x.TimeThreshold(); err != nil {
				return u, has, fmt.Errorf("URR %d Time Threshold: %w", id, err)
			}

			has.timeThreshold = true

		case ie.VolumeQuota:
			var f *ie.VolumeQuotaFields
			if f, err = x.VolumeQuota(); err != nil {
				return u, has, fmt.Errorf("URR %d Volume Quota: %w", id, err)
			}

			if f.Flags&volumeFlags == 0 {
				return u, has, fmt.Errorf("URR %d Volume Quota: %w", id, errURRNoFlagSet)
			}

			has.volumeQuota = true
			u.volumeQuota = volumeLimit{
				total: f.TotalVolume, uplink: f.UplinkVolume, downlink: f.DownlinkVolume,
				hasTotal: f.HasTOVOL(), hasUplink: f.HasULVOL(), hasDownlink: f.HasDLVOL(),
			}
		}
	}

	if create && !has.measurementMethod {
		return u, has, fmt.Errorf("URR %d: %w", id, errURRMeasurementMethodMissing)
	}

	if create && !has.reportingTriggers {
		return u, has, fmt.Errorf("URR %d: %w", id, errURRReportingTriggersMissing)
	}

	return u, has, nil
}
