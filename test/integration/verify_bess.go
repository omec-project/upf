// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/omec-project/upf-epc/pkg/fake_bess"
	"github.com/stretchr/testify/require"
	"testing"
)

// TODO: current assertions are limited to quantity verification only. We'd like to extend this
//       and check entry contents as well.
func verifyBessEntries(t *testing.T, bess *fake_bess.FakeBESS, testdata *pfcpSessionData, expectedValues p4RtValues, ueState UEState) {
	// Check we have all expected PDRs.
	pdrs := bess.GetPdrTableEntries()
	require.Equal(t, len(expectedValues.pdrs), len(pdrs), "found unexpected PDR entries %v", pdrs)
	for _, expectedPdr := range expectedValues.pdrs {
		id, err := expectedPdr.PDRID()
		require.NoError(t, err)

		_, found := pdrs[uint32(id)]
		require.True(t, found, "missing PDR", "expected PDR with ID %v: %+v, got %+v", id, expectedPdr, pdrs)
	}

	// Check we have all expected FARs.
	fars := bess.GetFarTableEntries()
	require.Equal(t, len(expectedValues.pdrs), len(fars), "found unexpected FAR entries %v", fars)
	for _, expectedFar := range expectedValues.fars {
		id, err := expectedFar.FARID()
		require.NoError(t, err)

		_, found := fars[id]
		require.True(t, found, "missing FAR", "expected FAR with ID %v: %+v, got %+v", id, expectedFar, fars)
	}

	// Check we have all expected session and app QERs.
	qers := append(bess.GetSessionQerTableEntries(), bess.GetAppQerTableEntries()...)
	require.Equal(t, len(expectedValues.qers)*2 /* up and down link */, len(qers), "found unexpected QER entries %v", qers)
}

func verifyNoBessRuntimeEntries(t *testing.T, bess *fake_bess.FakeBESS) {
	pdrs := bess.GetPdrTableEntries()
	require.Equal(t, 0, len(pdrs), "found unexpected PDR entries: %v", pdrs)
	fars := bess.GetFarTableEntries()
	require.Equal(t, 0, len(fars), "found unexpected FAR entries: %v", fars)
	sessionQers := bess.GetSessionQerTableEntries()
	require.Equal(t, 0, len(sessionQers), "found unexpected session QER entries: %v", sessionQers)
	appQers := bess.GetAppQerTableEntries()
	require.Equal(t, 0, len(appQers), "found unexpected app QER entries: %v", appQers)
}
