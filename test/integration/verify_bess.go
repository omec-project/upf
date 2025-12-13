// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"testing"

	"github.com/omec-project/upf-epc/pkg/fake_bess"
)

// TODO: current assertions are limited to quantity verification only. We'd like to extend this
// and check entry contents as well.
func verifyBessEntries(t *testing.T, bess *fake_bess.FakeBESS, expectedValues ueSessionConfig) {
	// Check we have all expected PDRs.
	pdrs := bess.GetPdrTableEntries()
	if len(pdrs) != len(expectedValues.pdrs) {
		t.Errorf("PDR entries count mismatch. got = %d, want = %d (found unexpected PDR entries %v)",
			len(pdrs), len(expectedValues.pdrs), pdrs)
	}

	for _, expectedPdr := range expectedValues.pdrs {
		id, err := expectedPdr.PDRID()
		if err != nil {
			t.Fatalf("Failed to get PDR ID: %v", err)
		}

		_, found := pdrs[uint32(id)]
		if !found {
			t.Errorf("Missing PDR: expected PDR with ID %v: %+v, got %+v", id, expectedPdr, pdrs)
		}
	}

	// Check we have all expected FARs.
	fars := bess.GetFarTableEntries()
	if len(fars) != len(expectedValues.pdrs) {
		t.Errorf("FAR entries count mismatch. got = %d, want = %d (found unexpected FAR entries %v)",
			len(fars), len(expectedValues.pdrs), fars)
	}

	for _, expectedFar := range expectedValues.fars {
		id, err := expectedFar.FARID()
		if err != nil {
			t.Fatalf("Failed to get FAR ID: %v", err)
		}

		_, found := fars[id]
		if !found {
			t.Errorf("Missing FAR: expected FAR with ID %v: %+v, got %+v", id, expectedFar, fars)
		}
	}

	// Check we have all expected session and app QERs.
	qers := append(bess.GetSessionQerTableEntries(), bess.GetAppQerTableEntries()...)
	expectedQerCount := len(expectedValues.qers) * 2 // up and down link
	if len(qers) != expectedQerCount {
		t.Errorf("QER entries count mismatch. got = %d, want = %d (found unexpected QER entries %v)",
			len(qers), expectedQerCount, qers)
	}
}

func verifyNoBessRuntimeEntries(t *testing.T, bess *fake_bess.FakeBESS) {
	pdrs := bess.GetPdrTableEntries()
	if len(pdrs) != 0 {
		t.Errorf("Expected no PDR entries, but found %d: %v", len(pdrs), pdrs)
	}

	fars := bess.GetFarTableEntries()
	if len(fars) != 0 {
		t.Errorf("Expected no FAR entries, but found %d: %v", len(fars), fars)
	}

	sessionQers := bess.GetSessionQerTableEntries()
	if len(sessionQers) != 0 {
		t.Errorf("Expected no session QER entries, but found %d: %v", len(sessionQers), sessionQers)
	}

	appQers := bess.GetAppQerTableEntries()
	if len(appQers) != 0 {
		t.Errorf("Expected no app QER entries, but found %d: %v", len(appQers), appQers)
	}
}
