// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"net"
	"testing"
	"time"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/omec-project/upf-epc/test/integration/providers"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

var (
	pfcpClient *pfcpsim.PFCPClient
)

type testCase struct {
	input    *pfcpSessionData
	expected p4RtValues
	desc     string
}

func init() {
	if err := initMockUP4(); err != nil {
		panic("failed to initialize mock-up4")
	}

	providers.RunDockerCommand("pfcpiface", "/bin/pfcpiface -config /config.json")

	// wait for PFCP Agent to initialize
	time.Sleep(time.Second * 3)
}

func initMockUP4() (err error) {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 0, Low: 1})
	if err != nil {
		return err
	}
	defer providers.DisconnectP4rt()

	_, err = p4rtClient.SetFwdPipe("../../conf/p4/bin/bmv2.json", "../../conf/p4/bin/p4info.txt", 0)
	if err != nil {
		return err
	}

	ipAddr, err := conversion.IpToBinary(upfN3Address)
	if err != nil {
		return err
	}

	srcIface, err := conversion.UInt32ToBinary(srcIfaceAccess, 1)
	if err != nil {
		return err
	}

	direction, err := conversion.UInt32ToBinary(directionUplink, 1)
	if err != nil {
		return err
	}

	sliceID, err := conversion.UInt32ToBinary(defaultSliceID, 1)
	if err != nil {
		return err
	}

	te := p4rtClient.NewTableEntry("PreQosPipe.interfaces", []p4rtc.MatchInterface{&p4rtc.LpmMatch{
		Value: ipAddr,
		PLen:  32,
	}}, p4rtClient.NewTableActionDirect("PreQosPipe.set_source_iface",
		[][]byte{srcIface, direction, sliceID}), nil)

	if err := p4rtClient.InsertTableEntry(te); err != nil {
		return err
	}

	return nil
}

func setup(t *testing.T) {
	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	err := pfcpClient.ConnectN4("127.0.0.1")
	require.NoErrorf(t, err, "failed to connect to UPF")
}

func teardown(t *testing.T) {
	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}
}

func TestBasicPFCPAssociation(t *testing.T) {
	setup(t)
	defer teardown(t)

	err := pfcpClient.SetupAssociation()
	require.NoErrorf(t, err, "failed to setup PFCP association")

	time.Sleep(time.Second * 10)

	require.True(t, pfcpClient.IsAssociationAlive())
}

func TestSingleUEAttachAndDetach(t *testing.T) {
	setup(t)
	defer teardown(t)

	// Application filtering test cases
	testCases := []testCase{
		{
			input: &pfcpSessionData{
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out udp from any to assigned 80-80",
				ulTEID:       15,
				dlTEID:       16,
				sessQFI:      0x09,
				appQFI:       0x08,
			},
			expected: p4RtValues{
				appFilter: appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("0.0.0.0"),
					appPrefixLen: 0,
					appPort: portRange{
						80, 80,
					},
				},
				appID:        1,
				tunnelPeerID: 2,
			},
			desc: "APPLICATION FILTERING permit out udp from any to assigned 80-80",
		},
		{
			input: &pfcpSessionData{
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				// TODO: use wider port range once multi port ranges are supported
				sdfFilter: "permit out udp from 192.168.1.1/32 to assigned 80-80",
				ulTEID:    15,
				dlTEID:    16,
				sessQFI:   0x09,
				appQFI:    0x08,
			},
			expected: p4RtValues{
				appFilter: appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("192.168.1.1"),
					appPrefixLen: 32,
					appPort: portRange{
						80, 80,
					},
				},
				// FIXME: there is a dependency on previous test because pfcpiface doesn't clear application IDs properly
				//  See SDFAB-960
				appID:        2,
				tunnelPeerID: 2,
			},
			desc: "APPLICATION FILTERING permit out udp from 192.168.1.1/32 to assigned 80-80",
		},
		{
			input: &pfcpSessionData{
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out ip from any to assigned",
				ulTEID:       15,
				dlTEID:       16,
				sessQFI:      0x09,
				appQFI:       0x08,
			},
			expected: p4RtValues{
				// no application filtering rule expected
				appID:        0,
				tunnelPeerID: 2,
			},
			desc: "APPLICATION FILTERING ALLOW_ALL",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testUEAttachDetach(t, &tc)
		})
	}
}

func testUEAttachDetach(t *testing.T, testcase *testCase) {
	err := pfcpClient.SetupAssociation()
	require.NoErrorf(t, err, "failed to setup PFCP association")

	pdrs := []*ie.IE{
		session.NewPDRBuilder().MarkAsUplink().
			WithMethod(session.Create).
			WithID(1).
			WithTEID(testcase.input.ulTEID).
			WithN3Address(testcase.input.upfN3Address).
			WithSDFFilter(testcase.input.sdfFilter).
			WithFARID(1).
			AddQERID(4).
			AddQERID(1).BuildPDR(),
		session.NewPDRBuilder().MarkAsDownlink().
			WithMethod(session.Create).
			WithID(2).
			WithUEAddress(testcase.input.ueAddress).
			WithSDFFilter(testcase.input.sdfFilter).
			WithFARID(2).
			AddQERID(4).
			AddQERID(2).BuildPDR(),
	}

	fars := []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(1).WithDstInterface(ie.DstInterfaceCore).
			WithAction(ActionForward).BuildFAR(),
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionDrop).WithTEID(testcase.input.dlTEID).WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	qers := []*ie.IE{
		// session QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(4).WithQFI(testcase.input.sessQFI).
			WithUplinkMBR(500000).
			WithDownlinkMBR(500000).
			WithUplinkGBR(0).
			WithDownlinkGBR(0).Build(),
		// uplink application QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(1).WithQFI(testcase.input.appQFI).
			WithUplinkMBR(50000).
			WithDownlinkMBR(50000).
			WithUplinkGBR(30000).
			WithDownlinkGBR(30000).Build(),
		// downlink application QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(2).WithQFI(testcase.input.appQFI).
			WithUplinkMBR(50000).
			WithDownlinkMBR(50000).
			WithUplinkGBR(30000).
			WithDownlinkGBR(30000).Build(),
	}

	sess, err := pfcpClient.EstablishSession(pdrs, fars, qers)
	require.NoErrorf(t, err, "failed to establish PFCP session")

	verifyP4RuntimeEntries(t, testcase.input, testcase.expected, false)

	err = pfcpClient.ModifySession(sess, nil, []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithAction(ActionForward).WithDstInterface(ie.DstInterfaceAccess).
			WithTEID(testcase.input.dlTEID).WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}, nil)

	verifyP4RuntimeEntries(t, testcase.input, testcase.expected, true)

	err = pfcpClient.DeleteSession(sess)
	require.NoErrorf(t, err, "failed to delete PFCP session")

	err = pfcpClient.TeardownAssociation()
	require.NoErrorf(t, err, "failed to gracefully release PFCP association")

	verifyNoP4RuntimeEntries(t)

	// clear Tunnel Peers and Applications tables
	// FIXME: Temporary solution. They should be cleared by pfcpiface, see SDFAB-960
	p4rtClient, _ := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 2, Low: 1})
	defer providers.DisconnectP4rt()
	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.applications")
	for _, entry := range entries {
		p4rtClient.DeleteTableEntry(entry)
	}
}
