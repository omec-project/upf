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
	"strconv"
	"strings"
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

func buildExpectedApplicationsEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedAppID uint8) *p4_v1.TableEntry {
	fields := strings.Fields(testdata.sdfFilter)
	var proto uint8
	switch fields[2] {
	case "udp":
		proto = 0x11
	case "tcp":
		proto = 0x6
	case "icmp":
		proto = 0x1
	default:
		proto = 0
	}

	appPorts := strings.Split(fields[len(fields)-1], "-")
	low, _ := strconv.ParseUint(appPorts[0], 10, 16)
	var high uint64
	if len(appPorts) > 1 {
		high, _ = strconv.ParseUint(appPorts[1], 10, 16)
	} else {
		high = low
	}

	protoVal, _ := conversion.UInt32ToBinary(uint32(proto), 3)
	// TODO: we assume default SDF filter: permit out udp from any to assigned
	//  appIP, _ := conversion.IpToBinary("0.0.0.0")

	lowVal, _ := conversion.UInt32ToBinary(uint32(low), 1)
	highVal, _ := conversion.UInt32ToBinary(uint32(high), 1)

	appID, _ := conversion.UInt32ToBinary(uint32(expectedAppID), 3)

	te := client.NewTableEntry(TableApplications, []p4rtc.MatchInterface{
		&p4rtc.RangeMatch{
			Low:  conversion.ToCanonicalBytestring(lowVal),
			High: conversion.ToCanonicalBytestring(highVal),
		},
		&p4rtc.TernaryMatch{
			Value: protoVal,
			Mask:  []byte{0xff},
		},
	}, client.NewTableActionDirect(ActSetAppID, [][]byte{appID}), nil)
	te.Priority = int32(math.MaxUint8 - testdata.precedence)

	// p4runtime-go-client doesn't properly enumerate match fields
	// assuming "any" as application IP, we simply override FieldId
	// TODO: fix enumeration in p4runtime-go-client
	te.Match[0].FieldId = 2
	te.Match[1].FieldId = 3

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

func buildExpectedSessionsDownlinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedTunnelPeerID uint8) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(testdata.ueAddress)

	tunnelPeerID, _ := conversion.UInt32ToBinary(uint32(expectedTunnelPeerID), 3)

	return client.NewTableEntry(TableDownlinkSessions, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
	}, client.NewTableActionDirect(ActSetDownlinkSession, [][]byte{tunnelPeerID}), nil)
}

func buildExpectedTerminationsUplinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedAppID uint8) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(testdata.ueAddress)
	appID, _ := conversion.UInt32ToBinary(uint32(expectedAppID), 3)

	return client.NewTableEntry(TableUplinkTerminations, []p4rtc.MatchInterface{
		&p4rtc.ExactMatch{
			Value: ueAddr,
		},
		&p4rtc.ExactMatch{
			Value: appID,
		},
	}, client.NewTableActionDirect(ActUplinkTermFwdNoTC, [][]byte{}), nil)
}

func buildExpectedTerminationsDownlinkEntry(client *p4rtc.Client, testdata *pfcpSessionData, expectedAppID uint8, afterModification bool) *p4_v1.TableEntry {
	ueAddr, _ := conversion.IpToBinary(testdata.ueAddress)
	appID, _ := conversion.UInt32ToBinary(uint32(expectedAppID), 3)

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
func verifyP4RuntimeEntries(t *testing.T, testdata *pfcpSessionData, afterModification bool) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 0, Low: 1})
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	var (
		expectedAppID              uint8 = 1
		expectedTunnelPeerID       uint8 = 0
		expectedNumberOfAllEntries       = 6
	)

	if afterModification {
		// new tunnel peer
		expectedNumberOfAllEntries++
		expectedTunnelPeerID = 2
	}

	allInstalledEntries, _ := p4rtClient.ReadTableEntryWildcard("")
	require.Equal(t, expectedNumberOfAllEntries, len(allInstalledEntries),
		fmt.Sprintf("UP4 should have exactly %v entries installed", expectedNumberOfAllEntries))

	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.applications")
	require.Equal(t, 1, len(entries), "PreQosPipe.applications should contain 1 entry")
	require.Equal(t, buildExpectedApplicationsEntry(p4rtClient, testdata, expectedAppID), entries[0], "PreQosPipe.applications does not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_uplink")
	require.Equal(t, 1, len(entries), "PreQosPipe.sessions_uplink should contain 1 entry")
	require.Equal(t, buildExpectedSessionsUplinkEntry(p4rtClient, testdata), entries[0], "PreQosPipe.sessions_uplink does not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.sessions_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.sessions_downlink should contain 1 entry")
	require.Equal(t, buildExpectedSessionsDownlinkEntry(p4rtClient, testdata, expectedTunnelPeerID), entries[0], "PreQosPipe.sessions_downlink does not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_uplink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_uplink should contain 1 entry")
	expected := buildExpectedTerminationsUplinkEntry(p4rtClient, testdata, expectedAppID)
	// we don't compare the entire object because counter ID is auto-generated by pfcpiface
	require.Equal(t, expected.Action.GetAction().ActionId, entries[0].Action.GetAction().ActionId, "PreQosPipe.terminations_uplink action does not equal expected")
	require.Equal(t, expected.Match, entries[0].Match, "PreQosPipe.terminations_uplink match fields do not equal expected")

	entries, _ = p4rtClient.ReadTableEntryWildcard("PreQosPipe.terminations_downlink")
	require.Equal(t, 1, len(entries), "PreQosPipe.terminations_downlink should contain 1 entry")
	expected = buildExpectedTerminationsDownlinkEntry(p4rtClient, testdata, expectedAppID, afterModification)
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
		require.Equal(t, buildExpectedTunnelPeersEntry(p4rtClient, testdata, expectedTunnelPeerID), entries[0],
			"PreQosPipe.tunnel_peers does not equal expected")
	}
}

func verifyNoP4RuntimeEntries(t *testing.T) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 0, Low: 1})
	require.NoErrorf(t, err, "failed to connect to P4Runtime server")
	defer providers.DisconnectP4rt()

	allInstalledEntries, _ := p4rtClient.ReadTableEntryWildcard("")
	// table entries for interfaces table are not removed by pfcpiface
	// FIXME: tunnel_peers and applications are not cleared on session deletion/association release
	//  See SDFAB-960
	require.Equal(t, 3, len(allInstalledEntries), "UP4 should have only 3 entry installed", allInstalledEntries)
}
