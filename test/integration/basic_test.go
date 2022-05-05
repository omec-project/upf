// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/omec-project/upf-epc/internal/p4constants"
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/wmnsk/go-pfcp/message"
	"net"
	"os"
	"testing"
	"time"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

func TestUPFBasedUeIPAllocation(t *testing.T) {
	// TODO: verify if UEIP bit is set in the UP Function Features of PFCP Association Response
	setup(t, ConfigUPFBasedIPAllocation)
	defer teardown(t)

	testcase := testCase{
		input: &pfcpSessionData{
			sliceID:      1,
			nbAddress:    nodeBAddress,
			upfN3Address: upfN3Address,
			sdfFilter:    "permit out udp from any 80-80 to assigned",
			ulTEID:       15,
			dlTEID:       16,
			QFI:          0x9,
		},
		expected: p4RtValues{
			// first IP address from pool configured in ue_ip_alloc.json
			ueAddress: "10.250.0.1",
			appFilter: appFilter{
				proto:        0x11,
				appIP:        net.ParseIP("0.0.0.0"),
				appPrefixLen: 0,
				appPort: portRange{
					80, 80,
				},
			},
			tc: 3,
		},
	}

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
	}

	fars := []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(1).WithDstInterface(ie.DstInterfaceCore).
			WithAction(ActionForward).BuildFAR(),
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionDrop).WithTEID(testcase.input.dlTEID).
			WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	err := pfcpClient.SendSessionEstablishmentRequest(pdrs, fars, nil)
	require.NoError(t, err)

	resp, err := pfcpClient.PeekNextResponse()
	require.NoError(t, err)

	estResp, ok := resp.(*message.SessionEstablishmentResponse)
	require.True(t, ok)

	testcase.expected.pdrs = pdrs
	testcase.expected.fars = fars

	remoteSEID, err := estResp.UPFSEID.FSEID()
	require.NoError(t, err)

	// the PFCP response should contain exactly 1 Create PDR IE
	require.Len(t, estResp.CreatedPDR, 1)

	// verify if UE Address IE is provided and contains expected IP address
	ueIPs, err := estResp.CreatedPDR[0].UEIPAddress()
	require.NoError(t, err)

	require.Equal(t, net.ParseIP(testcase.expected.ueAddress).To4(), ueIPs.IPv4Address.To4())

	verifyEntries(t, testcase.input, testcase.expected, UEStateAttaching)

	// no need to send modification request, we can delete PFCP session

	err = pfcpClient.SendSessionDeletionRequest(0, remoteSEID.SEID)
	require.NoError(t, err)

	_, err = pfcpClient.PeekNextResponse()
	require.NoError(t, err)

	verifyNoEntries(t, testcase.expected)
}

func TestDetectUP4Restart(t *testing.T) {
	if !isDatapathUP4() {
		t.Skipf("Skipping UP4-specific test for datapath: %s", os.Getenv(EnvDatapath))
	}

	run := func(t *testing.T) {
		// restart UP4, it will close P4Runtime channel between pfcpiface and mock-up4
		MustStopMockUP4()
		MustStartMockUP4()

		// establish session, it forces pfcpiface to re-connect to UP4.
		// Otherwise, we would need to wait about 2 minutes for pfcpiface to re-connect.
		pfcpClient.EstablishSession([]*ie.IE{
			session.NewPDRBuilder().MarkAsUplink().
				WithMethod(session.Create).
				WithID(1).
				WithTEID(15).
				WithN3Address(upfN3Address).
				WithFARID(1).
				AddQERID(1).BuildPDR(),
			session.NewPDRBuilder().MarkAsDownlink().
				WithMethod(session.Create).
				WithID(2).
				WithUEAddress(ueAddress).
				WithFARID(2).
				AddQERID(1).BuildPDR(),
		}, []*ie.IE{
			session.NewFARBuilder().
				WithMethod(session.Create).WithID(1).WithDstInterface(ie.DstInterfaceCore).
				WithAction(ActionForward).BuildFAR(),
			session.NewFARBuilder().
				WithMethod(session.Create).WithID(2).
				WithDstInterface(ie.DstInterfaceAccess).
				WithAction(ActionDrop).WithTEID(16).
				WithDownlinkIP(nodeBAddress).BuildFAR(),
		}, []*ie.IE{
			session.NewQERBuilder().WithMethod(session.Create).WithID(1).
				WithUplinkMBR(500000).
				WithDownlinkMBR(500000).
				WithUplinkGBR(0).
				WithDownlinkGBR(0).Build(),
		})
	}

	t.Run("Do not clear on UP4 restart", func(t *testing.T) {
		setup(t, ConfigDefault)
		defer teardown(t)

		run(t)
		// do not clear state on UP4 restart means that interfaces will not be re-installed.
		// The assumption is that the ONOS cluster preserves them, but BMv2 doesn't.
		verifyNumberOfEntries(t, p4constants.TablePreQosPipeInterfaces, 0)
	})

	t.Run("Clear on UP4 restart", func(t *testing.T) {
		setup(t, ConfigWipeOutOnUP4Restart)
		defer teardown(t)

		run(t)
		// clear state on UP4 restart means that interfaces entries will be re-installed.
		verifyNumberOfEntries(t, p4constants.TablePreQosPipeInterfaces, 2)
	})
}

func TestPFCPHeartbeats(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	time.Sleep(time.Second * 10)

	// Heartbeats interval is 5 seconds by default.
	// If the association is alive after 10 seconds it means that PFCP Agent handles heartbeats properly.
	require.True(t, pfcpClient.IsAssociationAlive())
}

func TestSingleUEAttachAndDetach(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	// Application filtering test cases
	testCases := []testCase{
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out udp from any 80-80 to assigned",
				ulTEID:       15,
				dlTEID:       16,
				QFI:          0x9,
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
				tc: 3,
			},
			desc: "APPLICATION FILTERING permit out udp from any 80-80 to assigned",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out udp from 192.168.1.1/32 to assigned 80-100",
				ulTEID:       15,
				dlTEID:       16,
				QFI:          0x9,
			},
			expected: p4RtValues{
				appFilter: appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("192.168.1.1"),
					appPrefixLen: 32,
					appPort: portRange{
						80, 100,
					},
				},
				tc: 3,
			},
			desc: "APPLICATION FILTERING permit out udp from 192.168.1.1/32 to assigned 80-100",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    "permit out ip from any to assigned",
				ulTEID:       15,
				dlTEID:       16,
				QFI:          0x9,
			},
			expected: p4RtValues{
				// no application filtering rule expected
				tc: 3,
			},
			desc: "APPLICATION FILTERING ALLOW_ALL",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    defaultSDFFilter,
				ulTEID:       15,
				dlTEID:       16,

				QFI:              0x11,
				uplinkAppQerID:   1,
				downlinkAppQerID: 2,
				sessQerID:        4,
				sessGBR:          0,
				sessMBR:          500000,
				appGBR:           30000,
				appMBR:           50000,
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
				tc: 3,
			},
			desc: "QER_METERING - 1 session QER, 2 app QERs",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    defaultSDFFilter,
				ulTEID:       15,
				dlTEID:       16,
				QFI:          0x11,

				// indicates 5G case (no application QERs, only session QER)
				uplinkAppQerID:   0,
				downlinkAppQerID: 0,

				sessQerID: 4,
				sessGBR:   300000,
				sessMBR:   500000,
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
				tc: 3,
			},
			desc: "QER_METERING - session QER only",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    defaultSDFFilter,
				ulTEID:       15,
				dlTEID:       16,

				QFI:              0x08,
				uplinkAppQerID:   1,
				downlinkAppQerID: 2,
				sessQerID:        4,
				sessGBR:          0,
				sessMBR:          500000,
				appGBR:           30000,
				appMBR:           50000,
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
				tc: 2,
			},
			desc: "QER_METERING - TC for QFI",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    defaultSDFFilter,
				ulTEID:       15,
				dlTEID:       16,

				QFI:              0x08,
				uplinkAppQerID:   1,
				downlinkAppQerID: 2,
				sessQerID:        4,
				sessGBR:          0,
				sessMBR:          500000,
				appGBR:           30000,
				appMBR:           50000,
				ulGateClosed:     true,
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
				tc: 2,
			},
			desc: "QER UL gating",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    nodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: upfN3Address,
				sdfFilter:    defaultSDFFilter,
				ulTEID:       15,
				dlTEID:       16,

				QFI:              0x08,
				uplinkAppQerID:   1,
				downlinkAppQerID: 2,
				sessQerID:        4,
				sessGBR:          0,
				sessMBR:          500000,
				appGBR:           30000,
				appMBR:           50000,
				dlGateClosed:     true,
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
				tc: 2,
			},
			desc: "QER DL gating",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testUEAttachDetach(t, fillExpected(&tc))
		})
	}
}

func TestUEBuffering(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	tc := testCase{
		input: &pfcpSessionData{
			sliceID:      1,
			nbAddress:    nodeBAddress,
			ueAddress:    ueAddress,
			upfN3Address: upfN3Address,
			sdfFilter:    "permit out udp from any 80-80 to assigned",
			ulTEID:       15,
			dlTEID:       16,
			QFI:          0x9,
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
			tc: 3,
		},
	}

	testUEAttach(t, fillExpected(&tc))
	testUEBuffer(t, fillExpected(&tc))
	testUEDetach(t, fillExpected(&tc))
}

func TestSliceMeter(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	testCases := []testCase{
		{
			sliceConfig: &pfcpiface.NetworkSlice{
				SliceName: "P4-UPF-1",
				SliceQos: pfcpiface.SliceQos{
					UplinkMbr:    20000,
					UlBurstBytes: 10000,
					DownlinkMbr:  10000,
					DlBurstBytes: 10000,
					BitrateUnit:  "Kbps",
				},
			},
			expected: p4RtValues{
				sliceMeter: &sliceMeter{
					sliceID: 1,
					TC:      3,
					rate:    20000000,
					burst:   10000,
				},
			},
			desc: "Uplink rate higher",
		},
		{
			sliceConfig: &pfcpiface.NetworkSlice{
				SliceName: "P4-UPF-1",
				SliceQos: pfcpiface.SliceQos{
					UplinkMbr:    5000,
					UlBurstBytes: 10000,
					DownlinkMbr:  10000,
					DlBurstBytes: 10000,
					BitrateUnit:  "Kbps",
				},
			},
			expected: p4RtValues{
				sliceMeter: &sliceMeter{
					sliceID: 1,
					TC:      3,
					rate:    10000000,
					burst:   10000,
				},
			},
			desc: "Downlink rate higher",
		},
	}

	t.Run("No Slice Meters", func(t *testing.T) {
		verifySliceMeter(t, p4RtValues{})
	})

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testSliceMeter(t, &tc)
		})
	}
}

func fillExpected(tc *testCase) *testCase {
	if tc.expected.ueAddress == "" {
		tc.expected.ueAddress = tc.input.ueAddress
	}

	return tc
}

func testUEAttach(t *testing.T, testcase *testCase) {
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
			WithAction(ActionDrop).WithTEID(testcase.input.dlTEID).
			WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	var qers []*ie.IE
	if testcase.input.sessQerID != 0 {
		qers = append(qers,
			// session QER
			session.NewQERBuilder().WithMethod(session.Create).WithID(testcase.input.sessQerID).
				WithQFI(testcase.input.QFI).
				WithUplinkMBR(500000).
				WithDownlinkMBR(500000).
				WithUplinkGBR(0).
				WithDownlinkGBR(0).Build())
	}

	if testcase.input.uplinkAppQerID != 0 {
		gateStatus := ie.GateStatusOpen
		if testcase.input.ulGateClosed {
			gateStatus = ie.GateStatusClosed
		}
		qers = append(qers,
			// uplink application QER
			ie.NewCreateQER(
				ie.NewQERID(testcase.input.uplinkAppQerID),
				ie.NewQFI(testcase.input.QFI),
				ie.NewGateStatus(gateStatus, gateStatus),
				ie.NewMBR(testcase.input.appMBR, testcase.input.appMBR),
				ie.NewGBR(testcase.input.appGBR, testcase.input.appGBR),
			))
	}

	if testcase.input.downlinkAppQerID != 0 {
		gateStatus := ie.GateStatusOpen
		if testcase.input.dlGateClosed {
			gateStatus = ie.GateStatusClosed
		}
		qers = append(qers,
			// downlink application QER
			ie.NewCreateQER(
				ie.NewQERID(testcase.input.downlinkAppQerID),
				ie.NewQFI(testcase.input.QFI),
				ie.NewGateStatus(gateStatus, gateStatus),
				ie.NewMBR(testcase.input.appMBR, testcase.input.appMBR),
				ie.NewGBR(testcase.input.appGBR, testcase.input.appGBR),
			))
	}

	sess, err := pfcpClient.EstablishSession(pdrs, fars, qers)
	testcase.expected.pdrs = pdrs
	testcase.expected.fars = fars
	testcase.expected.qers = qers
	require.NoErrorf(t, err, "failed to establish PFCP session")
	testcase.session = sess

	verifyEntries(t, testcase.input, testcase.expected, UEStateAttaching)

	err = pfcpClient.ModifySession(sess, nil, []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithAction(ActionForward).WithDstInterface(ie.DstInterfaceAccess).
			WithTEID(testcase.input.dlTEID).WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}, nil)

	verifyEntries(t, testcase.input, testcase.expected, UEStateAttached)
}

func testUEBuffer(t *testing.T, testcase *testCase) {
	// start buffering
	fars := []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionBuffer | ActionNotify).WithTEID(0).
			WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	err := pfcpClient.ModifySession(testcase.session, nil, fars, nil)
	require.NoError(t, err)

	verifyEntries(t, testcase.input, testcase.expected, UEStateBuffering)

	// stop buffering
	fars = []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionForward).WithTEID(testcase.input.dlTEID).
			WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	err = pfcpClient.ModifySession(testcase.session, nil, fars, nil)
	require.NoError(t, err)

	verifyEntries(t, testcase.input, testcase.expected, UEStateAttached)
}

func testUEDetach(t *testing.T, testcase *testCase) {
	err := pfcpClient.DeleteSession(testcase.session)
	require.NoErrorf(t, err, "failed to delete PFCP session")

	verifyNoEntries(t, testcase.expected)
}

func testUEAttachDetach(t *testing.T, testcase *testCase) {
	testUEAttach(t, testcase)
	testUEDetach(t, testcase)

	if isDatapathUP4() {
		// re-initialize counters so that we can verify if pfcp-agent clears them properly
		mustInitCountersWithDummyValue()
	}
}

func testSliceMeter(t *testing.T, testcase *testCase) {
	if isDatapathUP4() {
		err := PushSliceMeterConfig(*testcase.sliceConfig)
		if err != nil {
			t.Error("Error when pushing slice meter config via REST APIs", err)
		}

		verifySliceMeter(t, testcase.expected)
	} else {
		t.Skip("TODO: implement slice meter test for BESS")
	}
}
