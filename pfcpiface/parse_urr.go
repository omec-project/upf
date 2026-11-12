// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"fmt"
	"time"

	"github.com/omec-project/upf-epc/logger"
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

// volumeLimit is a Volume Threshold (clause 8.2.13): a limit for each counter the control
// plane set.
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

// urrTriggers are the reporting triggers (clause 8.2.19) this UPF acts on. A URR that sets
// another is refused.
type urrTriggers struct {
	periodic        bool // PERIO
	volumeThreshold bool // VOLTH
	timeThreshold   bool // TIMTH
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

	usage urrUsage
}

func (u urr) String() string {
	return fmt.Sprintf("URR(id=%d, volume=%v, duration=%v, triggers=%+v, period=%v, "+
		"volumeThreshold=%+v, timeThreshold=%v)",
		u.urrID, u.measureVolume, u.measureDuration, u.triggers, u.measurementPeriod,
		u.volumeThreshold, u.timeThreshold)
}

// urrIEs records which of a URR's optional IEs a Create or Update URR carried. An Update
// URR changes only what it carries (clause 7.5.4.4).
type urrIEs struct {
	measurementMethod bool
	reportingTriggers bool
	measurementPeriod bool
	volumeThreshold   bool
	timeThreshold     bool
}

var (
	errURRMeasurementMethodMissing = errors.New("create URR without the mandatory Measurement Method IE")
	errURRReportingTriggersMissing = errors.New("create URR without the mandatory Reporting Triggers IE")
	errURRNoFlagSet                = errors.New("no flag set, where at least one shall be")

	// errURRNotSupported refuses provisioning this UPF would accept and not act on, with
	// "Service not supported" (clause 8.2.1).
	errURRNotSupported = errors.New("not supported by this UPF")
)

const (
	// The flags of Measurement Method (clause 8.2.40) and Volume Threshold (clause 8.2.13),
	// of which at least one shall be set.
	measurementMethodFlags = 0x07 // DURAT, VOLUM, EVENT
	volumeFlags            = 0x07 // TOVOL, ULVOL, DLVOL

	measurementMethodEVENT = 0x04

	// The Reporting Triggers this UPF acts on: PERIO, VOLTH and TIMTH, in octet 5
	// (clause 8.2.19). Octet 7 has two triggers and spare bits.
	reportingTriggersActedOn = 0x07
	reportingTriggersOctet7  = 0x03
)

// parseURR reads a Create URR or Update URR IE. A Create URR must carry a Measurement
// Method and Reporting Triggers (Table 7.5.2.4-1); an Update URR may leave out any IE but
// the URR ID. Either is refused if it carries an IE, a trigger or a measurement method
// this UPF does not act on (errURRNotSupported).
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

			if m&measurementMethodEVENT != 0 {
				return u, has, fmt.Errorf("URR %d Measurement Method EVENT: %w", id, errURRNotSupported)
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
			// octets.
			octet7 := byte(0)
			if len(octets) > 2 {
				octet7 = octets[2] & reportingTriggersOctet7
			}

			if octets[0] == 0 && octets[1] == 0 && octet7 == 0 {
				return u, has, fmt.Errorf("URR %d Reporting Triggers: %w", id, errURRNoFlagSet)
			}

			if octets[0]&^reportingTriggersActedOn != 0 || octets[1] != 0 || octet7 != 0 {
				return u, has, fmt.Errorf("URR %d Reporting Triggers %x: %w", id, octets, errURRNotSupported)
			}

			has.reportingTriggers = true
			u.triggers = urrTriggers{
				periodic:        x.HasPERIO(),
				volumeThreshold: x.HasVOLTH(),
				timeThreshold:   x.HasTIMTH(),
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

		// Each of its flags asks for what this UPF does not do (clause 8.2.68), and it
		// sets at least one.
		case ie.MeasurementInformation:
			var mi uint8
			if mi, err = x.MeasurementInformation(); err != nil {
				return u, has, fmt.Errorf("URR %d Measurement Information: %w", id, err)
			}

			if mi == 0 {
				return u, has, fmt.Errorf("URR %d Measurement Information: %w", id, errURRNoFlagSet)
			}

			return u, has, fmt.Errorf("URR %d Measurement Information %#x: %w", id, mi, errURRNotSupported)

		// The other IEs of a Create or Update URR (Tables 7.5.2.4-1 and 7.5.4.4-1); FAR ID,
		// Application ID and SDF Filter are those of the Quota Action.
		case ie.VolumeQuota, ie.EventThreshold, ie.EventQuota, ie.TimeQuota, ie.QuotaHoldingTime,
			ie.DroppedDLTrafficThreshold, ie.QuotaValidityTime, ie.MonitoringTime,
			ie.SubsequentVolumeThreshold, ie.SubsequentTimeThreshold, ie.SubsequentVolumeQuota,
			ie.SubsequentTimeQuota, ie.SubsequentEventThreshold, ie.SubsequentEventQuota,
			ie.InactivityDetectionTime, ie.LinkedURRID, ie.TimeQuotaMechanism, ie.AggregatedURRs,
			ie.FARID, ie.EthernetInactivityTimer, ie.AdditionalMonitoringTime, ie.NumberOfReports,
			ie.ApplicationID, ie.SDFFilter, ie.UserPlaneInactivityTimer:
			return u, has, fmt.Errorf("URR %d IE type %d: %w", id, x.Type, errURRNotSupported)

		case ie.URRID: // read above

		// An IE of a type the URR does not define, or of a later release, is discarded
		// (clauses 7.6.1 and 7.6.9).
		default:
			logger.PfcpLog.Errorln("URR", id, "carries an unexpected IE of type", x.Type, "; it is discarded")
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
