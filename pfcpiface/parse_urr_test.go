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
		ie.NewReportingTriggers(triggerOctet5PERIO|triggerOctet5VOLTH|triggerOctet5TIMTH, triggerOctet6VOLQU),
		ie.NewMeasurementPeriod(30*time.Second),
		ie.NewVolumeThreshold(volumeTOVOL|volumeDLVOL, 8000, 0, 6000),
		ie.NewTimeThreshold(time.Minute),
		ie.NewVolumeQuota(volumeTOVOL|volumeULVOL, 10000, 4000, 0),
	), true)
	if err != nil {
		t.Fatalf("parseURR: %v", err)
	}

	want := urr{
		urrID:           testURRID,
		measureVolume:   true,
		measureDuration: true,
		triggers: urrTriggers{
			periodic: true, volumeThreshold: true, timeThreshold: true, volumeQuota: true,
		},
		measurementPeriod: 30 * time.Second,
		volumeThreshold:   volumeLimit{total: 8000, downlink: 6000, hasTotal: true, hasDownlink: true},
		timeThreshold:     time.Minute,
		volumeQuota:       volumeLimit{total: 10000, uplink: 4000, hasTotal: true, hasUplink: true},
	}

	if u != want {
		t.Fatalf("parsed\n%v\nexpected\n%v", u, want)
	}

	if has != (urrIEs{true, true, true, true, true, true}) {
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

// A URR whose threshold, quota or period cannot be read would be measured against limits
// the control plane did not set, so the message is refused instead.
func TestParseURRRefusesAMalformedLimit(t *testing.T) {
	for name, limit := range map[string]*ie.IE{
		"Volume Threshold":   ie.New(ie.VolumeThreshold, []byte{volumeTOVOL}), // flags but no volume
		"Volume Quota":       ie.New(ie.VolumeQuota, []byte{volumeTOVOL}),
		"Time Threshold":     ie.New(ie.TimeThreshold, []byte{0, 0}), // two of its four octets
		"Measurement Period": ie.New(ie.MeasurementPeriod, []byte{0, 0}),
	} {
		_, _, err := parseURR(ie.NewCreateURR(
			ie.NewURRID(testURRID),
			ie.NewMeasurementMethod(0, 1, 0),
			ie.NewReportingTriggers(0, triggerOctet6VOLQU),
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
