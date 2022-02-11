// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"fmt"

	"net"
	"testing"
	"time"

	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/omec-project/upf-epc/test/integration/providers"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

const (
	configDefault              = "default.json"
	configUPFBasedIPAllocation = "ue_ip_alloc.json"
)

var (
	pfcpClient *pfcpsim.PFCPClient
)

type testContext struct {
	UPFBasedUeIPAllocation bool
}

type testCase struct {
	ctx      testContext
	input    *pfcpSessionData
	expected p4RtValues

	desc string
}

func setup(t *testing.T, pfcpAgentConfig string) {
	providers.RunDockerCommandAttach("pfcpiface",
		fmt.Sprintf("/bin/pfcpiface -config /config/%s", pfcpAgentConfig))

	// wait for PFCP Agent to initialize, blocking
	err := waitForPFCPAgentToStart()
	require.NoErrorf(t, err, "failed to start PFCP Agent: %v", err)

	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	err = pfcpClient.ConnectN4("127.0.0.1")
	require.NoErrorf(t, err, "failed to connect to UPF")
}

func teardown(t *testing.T) {
	// clear Tunnel Peers table
	// FIXME: Temporary solution. They should be cleared by pfcpiface, see SDFAB-960
	p4rtClient, _ := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 2, Low: 1})
	defer providers.DisconnectP4rt()
	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.tunnel_peers")
	for _, entry := range entries {
		p4rtClient.DeleteTableEntry(entry)
	}

	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}

	// kill pfcpiface process inside container
	_, _, _, err := providers.RunDockerExecCommand("pfcpiface", "killall -9 pfcpiface")
	require.NoError(t, err, "failed to kill pfcpiface process")
}

func TestUPFBasedUeIPAllocation(t *testing.T) {
	setup(t, configUPFBasedIPAllocation)
	defer teardown(t)

	tc := testCase{
		ctx: testContext{
			UPFBasedUeIPAllocation: true,
		},
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
			// first IP address from pool configured in ue_ip_alloc.json
			ueAddress: "10.250.0.1",
			// no application filtering rule expected
			appID:        0,
			tunnelPeerID: 2,
		},
		desc: "UPF-based UE IP allocation",
	}

	t.Run(tc.desc, func(t *testing.T) {
		testUEAttachDetach(t, fillExpected(&tc))
	})
}

func TestBasicPFCPAssociation(t *testing.T) {
	setup(t, configDefault)
	defer teardown(t)

	err := pfcpClient.SetupAssociation()
	require.NoErrorf(t, err, "failed to setup PFCP association")

	time.Sleep(time.Second * 10)

	require.True(t, pfcpClient.IsAssociationAlive())
}

func TestSingleUEAttachAndDetach(t *testing.T) {
	setup(t, configDefault)
	defer teardown(t)

	// Application filtering test cases
	testCases := []testCase{
		{
			input: &pfcpSessionData{
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out udp from any 80-80 to assigned",
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
			desc: "APPLICATION FILTERING permit out udp from any 80-80 to assigned",
		},
		{
			input: &pfcpSessionData{
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out udp from 192.168.1.1/32 to assigned 80-400",
				ulTEID:       15,
				dlTEID:       16,
				sessQFI:      0x09,
				appQFI:       0x08,
			},
			expected: p4RtValues{
				appFilter: appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("192.168.1.1"),
					appPrefixLen: 32,
					appPort: portRange{
						80, 400,
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
			testUEAttachDetach(t, fillExpected(&tc))
		})
	}
}

func fillExpected(tc *testCase) *testCase {
	if tc.expected.ueAddress == "" {
		tc.expected.ueAddress = tc.input.ueAddress
	}

	return tc
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
	}

	if !testcase.ctx.UPFBasedUeIPAllocation {
		pdrs = append(pdrs,
			session.NewPDRBuilder().MarkAsDownlink().
				WithMethod(session.Create).
				WithID(2).
				WithUEAddress(testcase.input.ueAddress).
				WithSDFFilter(testcase.input.sdfFilter).
				WithFARID(2).
				AddQERID(4).
				AddQERID(2).BuildPDR())
	} else {
		pdrs = append(pdrs,
			// TODO: should be replaced by builder?
			ie.NewCreatePDR(
				ie.NewPDRID(2),
				ie.NewPrecedence(testcase.input.precedence),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					// indicate UP to allocate UE IP Address
					ie.NewUEIPAddress(0x10, "", "", 0, 0),
					ie.NewSDFFilter(testcase.input.sdfFilter, "", "", "", 1),
				),
				ie.NewFARID(2),
				ie.NewQERID(2),
				ie.NewQERID(4),
			),
		)
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

	// clear Applications table
	// FIXME: Temporary solution. They should be cleared by pfcpiface, see SDFAB-960
	p4rtClient, _ := providers.ConnectP4rt("127.0.0.1:50001", p4_v1.Uint128{High: 2, Low: 1})
	defer func() {
		providers.DisconnectP4rt()
		// give pfcpiface time to become master controller again
		time.Sleep(3 * time.Second)
	}()
	entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.applications")
	for _, entry := range entries {
		p4rtClient.DeleteTableEntry(entry)
	}
}
