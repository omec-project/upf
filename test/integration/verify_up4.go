// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"fmt"
	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	"github.com/omec-project/upf-epc/internal/p4constants"
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

const (
	// TODO: auto-generate P4 constants from P4Info and share them with p4rt_translator
	MeterSession              = "PreQosPipe.session_meter"
	MeterApp                  = "PreQosPipe.app_meter"
	TableApplications         = "PreQosPipe.applications"
	TableDownlinkSessions     = "PreQosPipe.sessions_downlink"
	TableSessionsUplink       = "PreQosPipe.sessions_uplink"
	TableUplinkTerminations   = "PreQosPipe.terminations_uplink"
	TableDownlinkTerminations = "PreQosPipe.terminations_downlink"
	TableTunnelPeers          = "PreQosPipe.tunnel_peers"
	ActSetAppID               = "PreQosPipe.set_app_id"
	ActSetUplinkSession       = "PreQosPipe.set_session_uplink"
	ActSetDownlinkSession     = "PreQosPipe.set_session_downlink"
	ActSetDownlinkSessionBuff = "PreQosPipe.set_session_downlink_buff"
	ActUplinkTermDrop         = "PreQosPipe.uplink_term_drop"
	ActUplinkTermFwd          = "PreQosPipe.uplink_term_fwd"
	ActLoadTunnelParam        = "PreQosPipe.load_tunnel_param"
	ActDownlinkTermDrop       = "PreQosPipe.downlink_term_drop"
	ActDownlinkTermFwd        = "PreQosPipe.downlink_term_fwd"
)

const (
	minGTPTunnelPeerID uint8 = 2
	maxGTPTunnelPeerID uint8 = 255
	minApplicationID   uint8 = 1
	maxApplicationID   uint8 = 255
)

var (
	tablesNames = p4constants.GetTableIDToNameMap()
	actionNames = p4constants.GetActionIDToNameMap()
)

func buildExpectedInterfacesEntries(client *p4rtc.Client, testdata *pfcpSessionData, expectedValues p4RtValues) []*p4_v1.TableEntry {
	entries := make([]*p4_v1.TableEntry, 0, 2)

	n3Addr, _ := conversion.IpToBinary(testdata.upfN3Address)

	te := client.NewTableEntry(tablesNames[p4constants.TablePreQosPipeInterfaces], []p4rtc.MatchInterface{
		&p4rtc.LpmMatch{
			Value: n3Addr,
			PLen:  32,
		},
	}, client.NewTableActionDirect(actionNames[p4constants.ActionPreQosPipeSetSourceIface],
		[][]byte{{directionUplink}, {srcIfaceAccess}, {testdata.sliceID}}),
		nil)

	entries = append(entries, te)

	ueAddr, _ := conversion.IpToBinary(expectedValues.ueAddress)

	te = client.NewTableEntry(tablesNames[p4constants.TablePreQosPipeInterfaces], []p4rtc.MatchInterface{
		&p4rtc.LpmMatch{
			Value: ueAddr,
			PLen:  16,
		},
	}, client.NewTableActionDirect(actionNames[p4constants.ActionPreQosPipeSetSourceIface],
		[][]byte{{directionDownlink}, {srcIfaceCore}, {testdata.sliceID}}),
		nil)

	entries = append(entries, te)

	return entries
}

func buildExpectedApplicationsEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedValues p4RtValues) *p4_v1.TableEntry {
	if expectedValues.appFilter.proto == 0 && len(expectedValues.appFilter.appIP) == 0 &&
		expectedValues.appFilter.appPort.low == 0 && expectedValues.appFilter.appPort.high == 0 {
		return nil
	}

	mfs := make([]p4rtc.MatchInterface, 0)

	mfs = append(mfs, &p4rtc.ExactMatch{
		Value: []byte{testdata.sliceID},
	})

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

	dummyAppID := 0x00
	appID, _ := conversion.UInt32ToBinary(uint32(dummyAppID), 3)

	te := client.NewTableEntry(TableApplications, mfs,
		client.NewTableActionDirect(ActSetAppID, [][]byte{appID}), nil)
	te.Priority = int32(math.MaxUint16 - testdata.precedence)

	// p4runtime-go-client doesn't properly enumerate match fields
	// TODO: fix enumeration in p4runtime-go-client
	for _, mf := range te.Match {
		if mf.GetLpm() != nil {
			mf.FieldId = 2
		}
		if mf.GetRange() != nil {
			mf.FieldId = 3
		}
		if mf.GetTernary() != nil {
			mf.FieldId = 4
		}
	}

	return te
}

func buildExpectedSessionsUplinkEntry(client *p4rtc.Client, testdata *pfcpSessionData) *p4_v1.TableEntry {
	n3Addr, _ := conversion.IpToBinary(testdata.upfN3Address)
	teid, _ := conversion.UInt32ToBinary(testdata.ulTEID, 3)

	// randomly generated by pfcpiface
	dummySessMeterIndex := []byte{0x00, 0x00}

	return client.NewTableEntry(TableSessionsUplink, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: n3Addr,
		},
		&p4rtc.ExactMatch{
			Value: teid,
		},
	}, client.NewTableActionDirect(ActSetUplinkSession, [][]byte{dummySessMeterIndex}), nil)
}

func buildExpectedSessionsDownlinkEntry(client *p4rtc.Client, expected p4RtValues, ueState UEState) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)

	// randomly generated by pfcpiface
	dummyTunnelPeerID := []byte{0x00}
	dummySessMeterIndex := []byte{0x00, 0x00, 0x00, 0x00}

	var action string
	var actionParams [][]byte
	switch ueState {
	case UEStateAttaching, UEStateAttached:
		action = ActSetDownlinkSession
		actionParams = [][]byte{dummyTunnelPeerID, dummySessMeterIndex}
		break
	case UEStateBuffering:
		action = ActSetDownlinkSessionBuff
		actionParams = [][]byte{dummySessMeterIndex}
		break
	}

	return client.NewTableEntry(TableDownlinkSessions, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
	}, client.NewTableActionDirect(action, actionParams), nil)
}

func buildExpectedTerminationsUplinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expected p4RtValues) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)
	dummyAppID := 0x00
	appID, _ := conversion.UInt32ToBinary(uint32(dummyAppID), 3)

	action := ActUplinkTermDrop
	params := [][]byte{
		// dummy counter ID
		{0x00, 0x00, 0x00, 0x00},
	}

	if !testdata.ulGateClosed {
		action = ActUplinkTermFwd
		params = [][]byte{
			// dummy counter ID
			{0x00, 0x00, 0x00, 0x00},
			{expected.tc},
			// dummy app meter idx
			{0x00, 0x00},
		}
	}

	return client.NewTableEntry(TableUplinkTerminations, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
		&p4rtc.ExactMatch{
			Value: appID,
		},
	}, client.NewTableActionDirect(action, params), nil)
}

func buildExpectedTerminationsDownlinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expected p4RtValues, afterModification bool) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(expected.ueAddress)
	dummyAppID := 0x00
	appID, _ := conversion.UInt32ToBinary(uint32(dummyAppID), 3)

	var actionParams [][]byte
	action := ActDownlinkTermDrop

	// dummy counter id, ignored later
	actionParams = append(actionParams, []byte{0x00, 0x00, 0x00, 0x00})

	if afterModification && !testdata.dlGateClosed {
		action = ActDownlinkTermFwd

		teid, _ := conversion.UInt32ToBinary(testdata.dlTEID, 0)
		actionParams = append(actionParams, conversion.ToCanonicalBytestring(teid))

		qfi, _ := conversion.UInt32ToBinary(uint32(testdata.QFI), 3)
		actionParams = append(actionParams, conversion.ToCanonicalBytestring(qfi))

		actionParams = append(actionParams, []byte{expected.tc})

		// dummy app meter idx, ignored later
		actionParams = append(actionParams, []byte{0x00, 0x00})
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

func buildExpectedTunnelPeersEntry(client *p4rtc.Client, testdata *pfcpSessionData) *p4_v1.TableEntry {
	srcAddr, _ := conversion.IpToBinary(testdata.upfN3Address)
	dstAddr, _ := conversion.IpToBinary(testdata.nbAddress)
	srcPort, _ := conversion.UInt32ToBinary(2152, 0)
	srcPort = conversion.ToCanonicalBytestring(srcPort)

	return client.NewTableEntry(TableTunnelPeers, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			// dummy tunnel peer ID
			Value: []byte{0x00},
		},
	}, client.NewTableActionDirect(ActLoadTunnelParam, [][]byte{srcAddr, dstAddr, srcPort}), nil)
}

func buildExpectedSliceTcMeter(expectedValues sliceMeter) (*p4_v1.MeterEntry, error) {
	meterIndex, err := pfcpiface.GetSliceTCMeterIndex(expectedValues.sliceID, expectedValues.TC)
	if err != nil {
		return nil, err
	}

	// slice_TC meters are expected to support only peak bands (Maximum BitRate)
	meterConfig := &p4_v1.MeterConfig{
		Cir:    int64(0),
		Cburst: int64(0),
		Pir:    expectedValues.rate,
		Pburst: expectedValues.burst,
	}

	return &p4_v1.MeterEntry{
		MeterId: 336833095,
		Index:   &p4_v1.Index{Index: meterIndex},
		Config:  meterConfig,
	}, nil
}

// TODO: we should pass a list of pfcpSessionData if we will test multiple UEs
func verifyP4RuntimeEntries(t *testing.T, testdata *pfcpSessionData, expectedValues p4RtValues, ueState UEState) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", false)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	var (
		expectedApplicationsEntries       = 1
		applicationID               uint8 = 0
	)

	expectedApplicationsEntry := buildExpectedApplicationsEntry(p4rtClient, testdata, expectedValues)
	if expectedApplicationsEntry == nil {
		expectedApplicationsEntries = 0
	}

	// FIXME: uncomment once pfcpiface properly removes all the state, see SDFAB-960
	//allInstalledEntries, _ := p4rtClient.ReadTableEntryWildcard("")
	//require.Equal(t, expectedNumberOfAllEntries, len(allInstalledEntries),
	//	fmt.Sprintf("UP4 should have exactly %v p4RtEntries installed", expectedNumberOfAllEntries),
	//	allInstalledEntries)

	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.interfaces")
	require.Equal(t, 2, len(entries))
	expectedInterfacesEntries := buildExpectedInterfacesEntries(p4rtClient, testdata, expectedValues)
	n3addressEntry := expectedInterfacesEntries[0]
	uePoolEntry := expectedInterfacesEntries[1]
	require.Contains(t, entries, n3addressEntry)
	require.Contains(t, entries, uePoolEntry)

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.applications")
	require.Equal(t, expectedApplicationsEntries, len(entries),
		fmt.Sprintf("PreQosPipe.applications should contain %v entry", expectedApplicationsEntries))
	if len(entries) > 0 && expectedApplicationsEntry != nil {
		require.Equal(t, expectedApplicationsEntry.Match, entries[0].Match)
		require.Equal(t, expectedApplicationsEntry.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId)
		require.GreaterOrEqual(t, entries[0].Action.GetAction().Params[0].Value[0], minApplicationID)
		require.LessOrEqual(t, entries[0].Action.GetAction().Params[0].Value[0], maxApplicationID)
		applicationID = entries[0].Action.GetAction().Params[0].Value[0]
	}

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_uplink")
	require.Equal(t, 1, len(entries), "PreQosPipe.sessions_uplink should contain 1 entry")
	expected := buildExpectedSessionsUplinkEntry(p4rtClient, testdata)
	require.Equal(t, expected.Match, entries[0].Match, "PreQosPipe.sessions_uplink does not equal expected")
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.sessions_uplink does not equal expected")
	require.Equal(t, len(expected.Action.GetAction().Params), len(entries[0].Action.GetAction().Params),
		"Number of action params for sessions_uplink does not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.sessions_downlink should contain 1 entry")
	expected = buildExpectedSessionsDownlinkEntry(p4rtClient, expectedValues, ueState)
	require.Equal(t, expected.Match, entries[0].Match, "PreQosPipe.sessions_downlink does not equal expected")
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.sessions_downlink does not equal expected")
	require.Equal(t, len(expected.Action.GetAction().Params), len(entries[0].Action.GetAction().Params),
		"Number of action params for sessions_downlink does not equal expected")

	var tunnelPeerSessionsDownlink []byte
	if ueState == UEStateAttached {
		// check if tunnel peer ID is within allowed range <2; 255>; session_meter_id is randomly generated
		require.LessOrEqual(t, entries[0].Action.GetAction().Params[0].Value[0], maxGTPTunnelPeerID)
		require.GreaterOrEqual(t, entries[0].Action.GetAction().Params[0].Value[0], minGTPTunnelPeerID)
		tunnelPeerSessionsDownlink = entries[0].Action.GetAction().Params[0].Value
	}

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_uplink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_uplink should contain 1 entry")
	expected = buildExpectedTerminationsUplinkEntry(p4rtClient, testdata, expectedValues)
	// we don't compare the entire object because counter ID is auto-generated by pfcpiface
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.terminations_uplink action does not equal expected")
	require.Equal(t, len(expected.Action.GetAction().Params), len(entries[0].Action.GetAction().Params),
		"Number of action params for terminations_uplink does not equal expected")
	require.Equal(t, applicationID, entries[0].Match[1].GetExact().Value[0])
	require.Equal(t, expected.Match[0], entries[0].Match[0], "PreQosPipe.terminations_uplink match fields do not equal expected")
	// check if counter index doesn't equal 0
	require.NotEqual(t, []byte{0}, entries[0].Action.GetAction().Params[0].Value)
	if !testdata.ulGateClosed {
		require.Equal(t, expected.Action.GetAction().Params[1], entries[0].Action.GetAction().Params[1], "PreQosPipe.terminations_uplink action params do not equal expected")
	}

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_downlink should contain 1 entry")
	expected = buildExpectedTerminationsDownlinkEntry(p4rtClient, testdata, expectedValues, ueState != UEStateAttaching)
	// we don't compare the entire object because counter ID is auto-generated by pfcpiface
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.terminations_downlink action does not equal expected")
	require.Equal(t, len(expected.Action.GetAction().Params), len(entries[0].Action.GetAction().Params),
		"Number of action params for terminations_downlink does not equal expected")
	// check if counter index doesn't equal 0
	require.NotEqual(t, []byte{0}, entries[0].Action.GetAction().Params[0].Value)
	// check app ID
	require.Equal(t, applicationID, entries[0].Match[1].GetExact().Value[0])
	// check UE address
	require.Equal(t, expected.Match[0], entries[0].Match[0], "PreQosPipe.terminations_downlink match fields do not equal expected")
	if ueState == UEStateAttached && !testdata.dlGateClosed {
		// ignore counter ID as it is random number generated by pfcpiface
		require.Equal(t, expected.Action.GetAction().Params[1], entries[0].Action.GetAction().Params[1],
			fmt.Sprintf("Action param (TEID) of action %v does not equal expected", ActDownlinkTermFwd))
		require.Equal(t, expected.Action.GetAction().Params[2], entries[0].Action.GetAction().Params[2],
			fmt.Sprintf("Action param (QFI) of action %v does not equal expected", ActDownlinkTermFwd))
		require.Equal(t, expected.Action.GetAction().Params[3], entries[0].Action.GetAction().Params[3],
			fmt.Sprintf("Action param (TC) of action %v does not equal expected", ActDownlinkTermFwd))
	}

	if ueState == UEStateAttached {
		entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.tunnel_peers")
		require.Equal(t, 1, len(entries), "PreQosPipe.tunnel_peers should contain 1 entry")
		expected := buildExpectedTunnelPeersEntry(p4rtClient, testdata)
		// check if tunnel peer id (match field) is within allowed range
		require.LessOrEqual(t, entries[0].Match[0].GetExact().Value[0], maxGTPTunnelPeerID)
		require.GreaterOrEqual(t, entries[0].Match[0].GetExact().Value[0], minGTPTunnelPeerID)
		require.Equal(t, expected.Action.GetAction(), entries[0].Action.GetAction())
		// check consistency between tunnel peers and sessions downlink
		require.Equal(t, tunnelPeerSessionsDownlink, entries[0].GetMatch()[0].GetExact().Value)
	}

	expectedNrOfConfiguredMeters := 2
	if testdata.downlinkAppQerID == 0 && testdata.uplinkAppQerID == 0 {
		// only session QER provided, pfcpiface shouldn't configure session meters
		expectedNrOfConfiguredMeters = 0
	}

	meters, _ := p4rtClient.ReadMeterEntryWildcard(MeterSession)
	nrOfConfiguredMeters := 0
	for _, m := range meters {
		if m.Config != nil {
			nrOfConfiguredMeters++
		}
	}
	require.Equal(t, expectedNrOfConfiguredMeters, nrOfConfiguredMeters,
		fmt.Sprintf("session meter should have %d cells configured", expectedNrOfConfiguredMeters))

	expectedNrOfConfiguredMeters = 2
	if testdata.downlinkAppQerID == 0 && testdata.uplinkAppQerID == 0 && testdata.sessQerID == 0 {
		// no QERs provided, pfcpiface shouldn't configure app meters
		expectedNrOfConfiguredMeters = 0
	}

	nrOfConfiguredMeters = 0
	meters, _ = p4rtClient.ReadMeterEntryWildcard(MeterApp)
	for _, m := range meters {
		if m.Config != nil {
			nrOfConfiguredMeters++
		}
	}
	require.Equal(t, expectedNrOfConfiguredMeters, nrOfConfiguredMeters,
		fmt.Sprintf("app meter should have %d cells configured", expectedNrOfConfiguredMeters))

	// we allocate & reset a Counter cell ID for each PDR
	expectedNrOfResetCounters := len(expectedValues.pdrs)

	verifyCounter := func(counterID uint32) {
		nrOfResetCounters := 0
		counters, _ := p4rtClient.ReadCounterEntryWildcard(p4constants.GetCounterIDToNameMap()[counterID])
		for _, c := range counters {
			if c.PacketCount == 0 && c.ByteCount == 0 {
				nrOfResetCounters++
			}
		}
		require.Equal(t, expectedNrOfResetCounters, nrOfResetCounters)
	}

	// verify ingress counter
	verifyCounter(p4constants.CounterPreQosPipePreQosCounter)
	// verify egress counter
	verifyCounter(p4constants.CounterPostQosPipePostQosCounter)
}

func verifyNumberOfEntries(t *testing.T, tableID uint32, expectedNoOfEntries int) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", false)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	entries, err := p4rtClient.ReadTableEntryWildcard(p4constants.GetTableIDToNameMap()[tableID])
	require.NoError(t, err)

	require.Len(t, entries, expectedNoOfEntries)
}

func verifyNoP4RuntimeEntries(t *testing.T, expectedValues p4RtValues) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", false)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	meters, _ := p4rtClient.ReadMeterEntryWildcard(MeterSession)
	nrOfConfiguredMeters := 0
	for _, m := range meters {
		if m.Config != nil {
			nrOfConfiguredMeters++
		}
	}
	require.Equal(t, 0, nrOfConfiguredMeters, "session meter should not have any cells configured")

	meters, _ = p4rtClient.ReadMeterEntryWildcard(MeterApp)
	nrOfConfiguredMeters = 0
	for _, m := range meters {
		if m.Config != nil {
			nrOfConfiguredMeters++
		}
	}
	require.Equal(t, 0, nrOfConfiguredMeters, "application meter should not have any cells configured")

	// 2 interfaces entries
	expectedAllEntries := 2

	allInstalledEntries, _ := p4rtClient.ReadTableEntryWildcard("")
	// table entries for interfaces table are not removed by pfcpiface
	require.Equal(t, expectedAllEntries, len(allInstalledEntries),
		fmt.Sprintf("UP4 should have only %d entry installed", expectedAllEntries), allInstalledEntries)

	tables := []string{
		tablesNames[p4constants.TablePreQosPipeApplications],
		tablesNames[p4constants.TablePreQosPipeTunnelPeers],
		tablesNames[p4constants.TablePreQosPipeSessionsUplink],
		tablesNames[p4constants.TablePreQosPipeSessionsDownlink],
		tablesNames[p4constants.TablePreQosPipeTerminationsUplink],
		tablesNames[p4constants.TablePreQosPipeTerminationsDownlink],
	}

	for _, table := range tables {
		entries, _ := p4rtClient.ReadTableEntryWildcard(table)
		require.Equal(t, 0, len(entries),
			fmt.Sprintf("%v should not contain any entries", table))
	}
}

func verifyP4RuntimeSliceMeter(t *testing.T, expectedValues p4RtValues) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", false)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	sliceTcMeter := p4constants.GetMeterIDToNameMap()[p4constants.MeterPreQosPipeSliceTcMeter]
	meters, _ := p4rtClient.ReadMeterEntryWildcard(sliceTcMeter)

	nrOfConfiguredMeters := 0
	for _, m := range meters {
		if m.Config != nil {
			nrOfConfiguredMeters++
		}
	}

	if expectedValues.sliceMeter != nil {
		expectedMeter, err := buildExpectedSliceTcMeter(*expectedValues.sliceMeter)
		if err != nil {
			t.Errorf("Error obtaining expected SliceTC meter: %v", err)
		}

		require.Equal(t, 1, nrOfConfiguredMeters, "A single slice TC meter is expected")

		meter, _ := p4rtClient.ReadMeterEntry(sliceTcMeter, expectedMeter.Index.GetIndex())
		require.Equal(t, expectedMeter.Config, meter, "Slice TC meter does not equal expected", meters)
	} else {
		require.Equal(t, 0, nrOfConfiguredMeters, "slice TC meter should not have any cells configured")
	}
}
