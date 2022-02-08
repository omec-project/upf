// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"fmt"
	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

const (
	// TODO: auto-generate P4 constants from P4Info and share them with p4rt_translator
	TableApplications         = "PreQosPipe.applications"
	TableDownlinkSessions     = "PreQosPipe.sessions_downlink"
	TableSessionsUplink       = "PreQosPipe.sessions_uplink"
	TableUplinkTerminations   = "PreQosPipe.terminations_uplink"
	TableDownlinkTerminations = "PreQosPipe.terminations_downlink"
	TableTunnelPeers          = "PreQosPipe.tunnel_peers"
	ActSetAppID               = "PreQosPipe.set_app_id"
	ActSetUplinkSession       = "PreQosPipe.set_session_uplink"
	ActSetDownlinkSession     = "PreQosPipe.set_session_downlink"
	ActUplinkTermFwdNoTC      = "PreQosPipe.uplink_term_fwd_no_tc"
	ActLoadTunnelParam        = "PreQosPipe.load_tunnel_param"
	ActDownlinkTermDrop       = "PreQosPipe.downlink_term_drop"
	ActDownlinkTermFwdNoTC    = "PreQosPipe.downlink_term_fwd_no_tc"
)

func buildExpectedApplicationsEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedValues p4RtValues) *p4_v1.TableEntry {
	if expectedValues.appFilter.proto == 0 && len(expectedValues.appFilter.appIP) == 0 &&
		expectedValues.appFilter.appPort.low == 0 && expectedValues.appFilter.appPort.high == 0 {
		return nil
	}

	mfs := make([]p4rtc.MatchInterface, 0)

	if len(expectedValues.appFilter.appIP) > 0 && !expectedValues.appFilter.appIP.IsUnspecified() {
		appIPVal, _ := conversion.IpToBinary(expectedValues.appFilter.appIP.String())
		mfs = append(mfs, &p4rtc.LpmMatch{
			Value: appIPVal,
			PLen:  int32(expectedValues.appFilter.appPrefixLen),
		})
	}

	if expectedValues.appFilter.appPort.low != 0 || expectedValues.appFilter.appPort.high != 0 {
		lowVal, _ := conversion.UInt32ToBinary(uint32(expectedValues.appFilter.appPort.low), 2)
		highVal, _ := conversion.UInt32ToBinary(uint32(expectedValues.appFilter.appPort.high), 2)
		mfs = append(mfs, &p4rtc.RangeMatch{
			Low:  conversion.ToCanonicalBytestring(lowVal),
			High: conversion.ToCanonicalBytestring(highVal),
		})
	}

	if expectedValues.appFilter.proto != 0 {
		protoVal, _ := conversion.UInt32ToBinary(uint32(expectedValues.appFilter.proto), 3)
		mfs = append(mfs, &p4rtc.TernaryMatch{
			Value: protoVal,
			Mask:  []byte{0xff},
		})
	}

	appID, _ := conversion.UInt32ToBinary(uint32(expectedValues.appID), 3)

	te := client.NewTableEntry(TableApplications, mfs,
		client.NewTableActionDirect(ActSetAppID, [][]byte{appID}), nil)
	te.Priority = int32(math.MaxUint16 - testdata.precedence)

	// p4runtime-go-client doesn't properly enumerate match fields
	// TODO: fix enumeration in p4runtime-go-client
	for _, mf := range te.Match {
		if mf.GetLpm() != nil {
			mf.FieldId = 1
		}
		if mf.GetRange() != nil {
			mf.FieldId = 2
		}
		if mf.GetTernary() != nil {
			mf.FieldId = 3
		}
	}

	return te
}

func buildExpectedSessionsUplinkEntry(client *p4rtc.Client, testdata *pfcpSessionData) *p4_v1.TableEntry {
	n3Addr, _ := conversion.IpToBinary(testdata.upfN3Address)
	teid, _ := conversion.UInt32ToBinary(testdata.ulTEID, 3)

	return client.NewTableEntry(TableSessionsUplink, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: n3Addr,
		},
		&p4rtc.ExactMatch{
			Value: teid,
		},
	}, client.NewTableActionDirect(ActSetUplinkSession, [][]byte{}), nil)
}

func buildExpectedSessionsDownlinkEntry(client *p4rtc.Client, expected p4RtValues) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)

	tunnelPeerID, _ := conversion.UInt32ToBinary(uint32(expected.tunnelPeerID), 3)

	return client.NewTableEntry(TableDownlinkSessions, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
	}, client.NewTableActionDirect(ActSetDownlinkSession, [][]byte{tunnelPeerID}), nil)
}

func buildExpectedTerminationsUplinkEntry(client *p4rtc.Client, expected p4RtValues) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)
	appID, _ := conversion.UInt32ToBinary(uint32(expected.appID), 3)

	return client.NewTableEntry(TableUplinkTerminations, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
		&p4rtc.ExactMatch{
			Value: appID,
		},
	}, client.NewTableActionDirect(ActUplinkTermFwdNoTC, [][]byte{}), nil)
}

func buildExpectedTerminationsDownlinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expected p4RtValues, afterModification bool) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)
	appID, _ := conversion.UInt32ToBinary(uint32(expected.appID), 3)

	var actionParams [][]byte
	action := ActDownlinkTermDrop

	if afterModification {
		action = ActDownlinkTermFwdNoTC
		// dummy counter id, ignored later
		actionParams = append(actionParams, []byte{0x00})

		teid, _ := conversion.UInt32ToBinary(testdata.dlTEID, 0)
		actionParams = append(actionParams, conversion.ToCanonicalBytestring(teid))

		// FIXME: QFI is not currently set
		actionParams = append(actionParams, []byte{0x00})
	}

	return client.NewTableEntry(TableDownlinkTerminations, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
		&p4rtc.ExactMatch{
			Value: appID,
		},
	}, client.NewTableActionDirect(action, actionParams), nil)
}

func buildExpectedTunnelPeersEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedTunnelPeerID uint8) *p4_v1.TableEntry {
	srcAddr, _ := conversion.IpToBinary(testdata.upfN3Address)
	dstAddr, _ := conversion.IpToBinary(testdata.nbAddress)
	srcPort, _ := conversion.UInt32ToBinary(2152, 0)
	srcPort = conversion.ToCanonicalBytestring(srcPort)

	return client.NewTableEntry(TableTunnelPeers, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: []byte{expectedTunnelPeerID},
		},
	}, client.NewTableActionDirect(ActLoadTunnelParam, [][]byte{srcAddr, dstAddr, srcPort}), nil)
}

// TODO: we should pass a list of pfcpSessionData if we will test multiple UEs
func verifyP4RuntimeEntries(t *testing.T, testdata *pfcpSessionData, expectedValues p4RtValues, afterModification bool) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", ReaderElectionID)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	var (
		expectedApplicationsEntries = 1
	)

	if !afterModification {
		// new tunnel peer
		expectedValues.tunnelPeerID = 0
	}

	expectedApplicationsEntry := buildExpectedApplicationsEntry(p4rtClient, testdata, expectedValues)
	if expectedApplicationsEntry == nil {
		expectedApplicationsEntries = 0
	}

	// FIXME: uncomment once pfcpiface properly removes all the state, see SDFAB-960
	//allInstalledEntries, _ := p4rtClient.ReadTableEntryWildcard("")
	//require.Equal(t, expectedNumberOfAllEntries, len(allInstalledEntries),
	//	fmt.Sprintf("UP4 should have exactly %v p4RtEntries installed", expectedNumberOfAllEntries),
	//	allInstalledEntries)

	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.applications")
	require.Equal(t, expectedApplicationsEntries, len(entries),
		fmt.Sprintf("PreQosPipe.applications should contain %v entry", expectedApplicationsEntries))
	if len(entries) > 0 {
		require.Equal(t, expectedApplicationsEntry, entries[0], "PreQosPipe.applications does not equal expected",
			entries)
	}

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_uplink")
	require.Equal(t, 1, len(entries),
		fmt.Sprintf("PreQosPipe.sessions_uplink should contain %v entries", 1))
	require.Equal(t, buildExpectedSessionsUplinkEntry(p4rtClient, testdata), entries[0], "PreQosPipe.sessions_uplink does not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.sessions_downlink should contain 1 entry")
	require.Equal(t, buildExpectedSessionsDownlinkEntry(p4rtClient, expectedValues), entries[0], "PreQosPipe.sessions_downlink does not equal expected")
	tunnelPeerSessionsDownlink := entries[0].Action.GetAction().Params[0].Value

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_uplink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_uplink should contain 1 entry")
	expected := buildExpectedTerminationsUplinkEntry(p4rtClient, expectedValues)
	// we don't compare the entire object because counter ID is auto-generated by pfcpiface
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.terminations_uplink action does not equal expected")
	require.Equal(t, expected.Match, entries[0].Match, "PreQosPipe.terminations_uplink match fields do not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_downlink should contain 1 entry")
	expected = buildExpectedTerminationsDownlinkEntry(p4rtClient, testdata, expectedValues, afterModification)
	// we don't compare the entire object because counter ID is auto-generated by pfcpiface
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.terminations_downlink action does not equal expected")
	require.Equal(t, expected.Match, entries[0].Match, "PreQosPipe.terminations_downlink match fields do not equal expected")
	if afterModification {
		// ignore counter ID as it is random number generated by pfcpiface
		require.Equal(t, expected.Action.GetAction().Params[1], entries[0].Action.GetAction().Params[1],
			fmt.Sprintf("Action param (TEID) of action %v does not equal expected", ActDownlinkTermFwdNoTC))
		require.Equal(t, expected.Action.GetAction().Params[2], entries[0].Action.GetAction().Params[2],
			fmt.Sprintf("Action param (QFI) of action %v does not equal expected", ActDownlinkTermFwdNoTC))
	}

	if afterModification {
		entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.tunnel_peers")
		require.Equal(t, 1, len(entries), "PreQosPipe.tunnel_peers should contain 1 entry")
		require.Equal(t, buildExpectedTunnelPeersEntry(p4rtClient, testdata, expectedValues.tunnelPeerID), entries[0],
			"PreQosPipe.tunnel_peers does not equal expected")
		// check consistency between tunnel peers and sessions downlink
		require.Equal(t, tunnelPeerSessionsDownlink, entries[0].GetMatch()[0].GetExact().Value)
	}
}

func verifyNoP4RuntimeEntries(t *testing.T) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", ReaderElectionID)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	tables := []string{
		// FIXME: tunnel_peers and applications are not cleared on session deletion/association release
		//  See SDFAB-960
		//  Add tunnel_peers and applications to the list, once fixed
		"PreQosPipe.sessions_uplink", "PreQosPipe.sessions_downlink",
		"PreQosPipe.terminations_uplink", "PreQosPipe.terminations_downlink",
	}

	for _, table := range tables {
		entries, _ := p4rtClient.ReadTableEntryWildcard(table)
		require.Equal(t, 0, len(entries),
			fmt.Sprintf("%v should not contain any entries", table))
	}
}
