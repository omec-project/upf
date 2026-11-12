// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
)

const (
	testURRID = 7

	// Reporting Triggers octet 5 and 6 bits, TS 29.244 clause 8.2.19.
	triggerOctet5PERIO = 0x01
	triggerOctet5VOLTH = 0x02
	triggerOctet5TIMTH = 0x04
	triggerOctet6VOLQU = 0x01

	// Volume flags, clauses 8.2.13 and 8.2.50.
	volumeTOVOL = 0x01
	volumeULVOL = 0x02
	volumeDLVOL = 0x04
)

func TestParseURRReadsWhatACreateURRProvisions(t *testing.T) {
	u, has, err := parseURR(ie.NewCreateURR(
		ie.NewURRID(testURRID),
		ie.NewMeasurementMethod(0, 1, 1),
		ie.NewReportingTriggers(triggerOctet5PERIO|triggerOctet5VOLTH|triggerOctet5TIMTH, 0),
		ie.NewMeasurementPeriod(30*time.Second),
		ie.NewVolumeThreshold(volumeTOVOL|volumeDLVOL, 8000, 0, 6000),
		ie.NewTimeThreshold(time.Minute),
		ie.New(9999, []byte{1}), // a type unknown to this release, discarded
	), true)
	if err != nil {
		t.Fatalf("parseURR: %v", err)
	}

	want := urr{
		urrID:           testURRID,
		measureVolume:   true,
		measureDuration: true,
		triggers: urrTriggers{
			periodic: true, volumeThreshold: true, timeThreshold: true,
		},
		measurementPeriod: 30 * time.Second,
		volumeThreshold:   volumeLimit{total: 8000, downlink: 6000, hasTotal: true, hasDownlink: true},
		timeThreshold:     time.Minute,
	}

	if u != want {
		t.Fatalf("parsed\n%v\nexpected\n%v", u, want)
	}

	if has != (urrIEs{true, true, true, true, true}) {
		t.Fatalf("expected every IE to be recorded as present, got %+v", has)
	}
}

// Measurement Method and Reporting Triggers are mandatory in a Create URR
// (Table 7.5.2.4-1), and nothing would be measured or reported for a URR without them.
func TestParseURRRefusesACreateURRWithoutItsMandatoryIEs(t *testing.T) {
	for _, tc := range []struct {
		name string
		urr  *ie.IE
		want error
	}{
		{
			"no Measurement Method",
			ie.NewCreateURR(ie.NewURRID(testURRID), ie.NewReportingTriggers(triggerOctet5PERIO, 0)),
			errURRMeasurementMethodMissing,
		},
		{
			"no Reporting Triggers",
			ie.NewCreateURR(ie.NewURRID(testURRID), ie.NewMeasurementMethod(0, 1, 0)),
			errURRReportingTriggersMissing,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseURR(tc.urr, true); !errors.Is(err, tc.want) {
				t.Fatalf("parseURR returned %v, expected %v", err, tc.want)
			}
		})
	}
}

// An Update URR carries only what changes (clause 7.5.4.4), so one with nothing but the
// URR ID is valid and records no IE as present.
func TestParseURRAcceptsAnUpdateURRThatChangesNothing(t *testing.T) {
	u, has, err := parseURR(ie.NewUpdateURR(ie.NewURRID(testURRID)), false)
	if err != nil {
		t.Fatalf("parseURR: %v", err)
	}

	if u.urrID != testURRID || has != (urrIEs{}) {
		t.Fatalf("got URR %d with %+v present, expected URR %d with nothing present",
			u.urrID, has, testURRID)
	}
}

// Measurement Method, Reporting Triggers, Volume Threshold and Measurement Information
// each set at least one flag (clauses 8.2.40, 8.2.19, 8.2.13 and 8.2.68). One that does not parse, or
// sets none, would turn measuring or reporting off, so the URR is refused instead.
func TestParseURRRefusesAFlagIEThatSetsNoFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		x    *ie.IE
		want error // nil: any error
	}{
		{"empty Measurement Method", ie.New(ie.MeasurementMethod, nil), nil},
		{"Measurement Method without a flag", ie.New(ie.MeasurementMethod, []byte{0}), errURRNoFlagSet},
		{"Measurement Method with only spare bits", ie.New(ie.MeasurementMethod, []byte{0xf8}), errURRNoFlagSet},
		{"one-octet Reporting Triggers", ie.New(ie.ReportingTriggers, []byte{triggerOctet5PERIO}), nil},
		{"Reporting Triggers without a trigger", ie.New(ie.ReportingTriggers, []byte{0, 0}), errURRNoFlagSet},
		{"Reporting Triggers with only spare bits", ie.New(ie.ReportingTriggers, []byte{0, 0, 0xfc}), errURRNoFlagSet},
		{"Volume Threshold without a flag", ie.New(ie.VolumeThreshold, []byte{0, 0}), errURRNoFlagSet},
		{"Measurement Information without a flag", ie.NewMeasurementInformation(0), errURRNoFlagSet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseURR(ie.NewUpdateURR(ie.NewURRID(testURRID), tc.x), false)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("parseURR returned %v, expected %v", err, tc.want)
			}
		})
	}
}

// A URR that asks for what this UPF does not do is refused, not accepted and ignored.
func TestParseURRRefusesWhatThisUPFDoesNotActOn(t *testing.T) {
	for name, x := range map[string]*ie.IE{
		"EVENT":                       ie.NewMeasurementMethod(1, 1, 0),
		"START":                       ie.NewReportingTriggers(triggerOctet5PERIO|0x10, 0),
		"VOLQU":                       ie.NewReportingTriggers(0, triggerOctet6VOLQU),
		"UPINT":                       ie.New(ie.ReportingTriggers, []byte{0, 0, 0x02}),
		"Volume Quota":                ie.NewVolumeQuota(volumeTOVOL, 1000, 0, 0),
		"Time Quota":                  ie.NewTimeQuota(time.Minute),
		"Quota Holding Time":          ie.NewQuotaHoldingTime(time.Minute),
		"Event Threshold":             ie.NewEventThreshold(10),
		"Monitoring Time":             ie.NewMonitoringTime(testEpoch),
		"Subsequent Volume Threshold": ie.NewSubsequentVolumeThreshold(volumeTOVOL, 1000, 0, 0),
		"Inactivity Detection Time":   ie.NewInactivityDetectionTime(30),
		"Linked URR ID":               ie.NewLinkedURRID(testURRID + 1),
		"Number of Reports":           ie.NewNumberOfReports(1),
		"Measurement Information":     ie.NewMeasurementInformation(0x02), // INAM
	} {
		var refused *urrNotActedOn

		_, _, err := parseURR(ie.NewUpdateURR(ie.NewURRID(testURRID), x), false)
		if !errors.As(err, &refused) || refused.urrID != testURRID {
			t.Errorf("an Update URR with %s returned %v, expected URR %d refused as not acted on", name, err, testURRID)
		}
	}
}

// A URR whose threshold or period cannot be read would be measured against limits
// the control plane did not set, so the message is refused instead.
func TestParseURRRefusesAMalformedLimit(t *testing.T) {
	for name, limit := range map[string]*ie.IE{
		"Volume Threshold":   ie.New(ie.VolumeThreshold, []byte{volumeTOVOL}), // flags but no volume
		"Time Threshold":     ie.New(ie.TimeThreshold, []byte{0, 0}),          // two of its four octets
		"Measurement Period": ie.New(ie.MeasurementPeriod, []byte{0, 0}),
	} {
		_, _, err := parseURR(ie.NewCreateURR(
			ie.NewURRID(testURRID),
			ie.NewMeasurementMethod(0, 1, 0),
			ie.NewReportingTriggers(triggerOctet5PERIO, 0),
			limit,
		), true)
		if err == nil {
			t.Errorf("a malformed %s was accepted", name)
		}
	}
}

func TestParsePDRReadsItsURRIDs(t *testing.T) {
	var p pdr

	err := p.parsePDR(corePDR(downlinkPDRID, firstUEAddress,
		ie.NewURRID(testURRID), ie.NewURRID(testURRID+1)), 1, nil, nil)
	if err != nil {
		t.Fatalf("parsePDR: %v", err)
	}

	if !p.hasURRIDs || len(p.urrIDList) != 2 || p.urrIDList[0] != testURRID || p.urrIDList[1] != testURRID+1 {
		t.Fatalf("expected URR IDs [%d %d], got %v (present %v)",
			testURRID, testURRID+1, p.urrIDList, p.hasURRIDs)
	}
}
