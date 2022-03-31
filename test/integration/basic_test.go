// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/wmnsk/go-pfcp/message"
	"github.com/omec-project/upf-epc/pfcpiface"
	"net"
	"testing"
	"time"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

func init() {
	if isDatapathUP4() && isModeDocker() {
		initForwardingPipelineConfig()
	}
}

func TestUPFBasedUeIPAllocation(t *testing.T) {
	// TODO: verify if UEIP bit is set in the UP Function Features of PFCP Association Response
	setup(t, ConfigUPFBasedIPAllocation)
	defer teardown(t)

	testcase := TestCase{
		input: NewTestInput().
			WithUE(), // single UE with default values
		expected: TestExpectations{
			// first IP address from pool configured in ue_ip_alloc.json
			ueAddress: "10.250.0.1",
		},
	}

	pdrs := []*ie.IE{
		session.NewPDRBuilder().MarkAsUplink().
			WithMethod(session.Create).
			WithID(1).
			WithTEID(testcase.input.UE.ulTEID).
			WithN3Address(testcase.input.UE.upfN3Address).
			WithSDFFilter(testcase.input.UE.sdfFilter).
			WithFARID(1).
			AddQERID(4).
			AddQERID(1).BuildPDR(),
		ie.NewCreatePDR(
			ie.NewPDRID(2),
			ie.NewPrecedence(testcase.input.UE.precedence),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				// indicate UP to allocate UE IP Address
				ie.NewUEIPAddress(0x10, "", "", 0, 0),
				ie.NewSDFFilter(testcase.input.UE.sdfFilter, "", "", "", 1),
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
			WithAction(ActionDrop).WithTEID(testcase.input.UE.dlTEID).
			WithDownlinkIP(testcase.input.UE.nbAddress).BuildFAR(),
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

	verifyEntries(t, testcase.input.UE, testcase.expected, UEStateAttaching)

	// no need to send modification request, we can delete PFCP session

	err = pfcpClient.SendSessionDeletionRequest(0, remoteSEID.SEID)
	require.NoError(t, err)

	_, err = pfcpClient.PeekNextResponse()
	require.NoError(t, err)

	verifyNoEntries(t, testcase.expected)
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
	testCases := []TestCase{
		{
			input: NewTestInput().WithUE(func(ueData *pfcpSessionData) {
				ueData.sdfFilter = "permit out udp from any 80-80 to assigned"
			}),
			expected: TestExpectations{
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
			input: NewTestInput().WithUE(func(ueData *pfcpSessionData) {
				ueData.sdfFilter = "permit out udp from 192.168.1.1/32 to assigned 80-100"
			}),
			expected: TestExpectations{
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
			input: NewTestInput().WithUE(func(ueData *pfcpSessionData) {
				ueData.sdfFilter = "permit out ip from any to assigned"
			}),
			expected: TestExpectations{
				// no application filtering rule expected
				tc: 3,
			},
			desc: "APPLICATION FILTERING ALLOW_ALL",
		},
		{
			input: NewTestInput().WithUE(func(ueData *pfcpSessionData) {
				ueData.sdfFilter = defaultSDFFilter
				ueData.QFI = 0x11
				ueData.sessGBR = 0
				ueData.sessMBR = 500000
				ueData.appGBR = 30000
				ueData.appMBR = 50000
				ueData.sessQerID = 4
				ueData.uplinkAppQerID = 1
				ueData.downlinkAppQerID = 2
			}),
			expected: NewTestExpectations(func(expect *TestExpectations) {
				expect.appFilter = appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("0.0.0.0"),
					appPrefixLen: 0,
					appPort: portRange{
						80, 80,
					},
				}
			}),
			desc: "QER_METERING - 1 session QER, 2 app QERs",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    defaultNodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: defaultUpfN3Address,
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
			expected: TestExpectations{
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
			input: NewTestInput().WithUE(func(ueData *pfcpSessionData) {
				ueData.QFI = 0x08

			}),

				&pfcpSessionData{
				sliceID:      1,
				nbAddress:    defaultNodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: defaultUpfN3Address,
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
			expected: NewTestExpectations(func(expect *TestExpectations) {
				expect.appFilter = appFilter{
					proto:        0x11,
					appIP:        net.ParseIP("0.0.0.0"),
					appPrefixLen: 0,
					appPort: portRange{
						80, 80,
					},
				}
				expect.tc = 2
			}),
			desc: "QER_METERING - TC for QFI",
		},
		{
			input: &pfcpSessionData{
				sliceID:      1,
				nbAddress:    defaultNodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: defaultUpfN3Address,
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
			expected: TestExpectations{
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
				nbAddress:    defaultNodeBAddress,
				ueAddress:    ueAddress,
				upfN3Address: defaultUpfN3Address,
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
			expected: TestExpectations{
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

	tc := &TestCase{
		input: NewTestInput().
			WithUE(),
		expected: TestExpectations{
			tc: 3,
		},
	}

	tc.Prepare()

	testUEAttach(t, tc)
	testUEBuffer(t, tc)
	testUEDetach(t, tc)
}

func TestSliceMeter(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	testCases := []TestCase{
		{
			input: NewTestInput().WithSliceConfig(&pfcpiface.NetworkSlice{
				SliceName: "P4-UPF-1",
				SliceQos: pfcpiface.SliceQos{
					UplinkMbr:    20000,
					UlBurstBytes: 10000,
					DownlinkMbr:  10000,
					DlBurstBytes: 10000,
					BitrateUnit:  "Kbps",
				}},
			),
			expected: TestExpectations{
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
			input: NewTestInput().WithSliceConfig(&pfcpiface.NetworkSlice{
				SliceName: "P4-UPF-1",
				SliceQos: pfcpiface.SliceQos{
					UplinkMbr:    5000,
					UlBurstBytes: 10000,
					DownlinkMbr:  10000,
					DlBurstBytes: 10000,
					BitrateUnit:  "Kbps",
				},
			}),
			expected: TestExpectations{
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
		verifySliceMeter(t, TestExpectations{})
	})

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testSliceMeter(t, &tc)
		})
	}
}

func testUEAttach(t *testing.T, testcase *TestCase) {
	getPDRs := func(data *pfcpSessionData) []*ie.IE {
		return []*ie.IE{
			session.NewPDRBuilder().MarkAsUplink().
				WithMethod(session.Create).
				WithID(1).
				WithTEID(data.ulTEID).
				WithN3Address(data.upfN3Address).
				WithSDFFilter(data.sdfFilter).
				WithFARID(1).
				AddQERID(4).
				AddQERID(1).BuildPDR(),
			session.NewPDRBuilder().MarkAsDownlink().
				WithMethod(session.Create).
				WithID(2).
				WithUEAddress(data.ueAddress).
				WithSDFFilter(data.sdfFilter).
				WithFARID(2).
				AddQERID(4).
				AddQERID(2).BuildPDR(),
		}
	}

	getFARs := func(data *pfcpSessionData) []*ie.IE {
		return []*ie.IE{
			session.NewFARBuilder().
				WithMethod(session.Create).WithID(1).WithDstInterface(ie.DstInterfaceCore).
				WithAction(ActionForward).BuildFAR(),
			session.NewFARBuilder().
				WithMethod(session.Create).WithID(2).
				WithDstInterface(ie.DstInterfaceAccess).
				WithAction(ActionDrop).WithTEID(data.dlTEID).
				WithDownlinkIP(data.nbAddress).BuildFAR(),
		}
	}

	pdrs := getPDRs(testcase.input.UE)
	fars := getFARs(testcase.input.UE)

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

func testUEBuffer(t *testing.T, testcase *TestCase) {
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

func testUEDetach(t *testing.T, testcase *TestCase) {
	err := pfcpClient.DeleteSession(testcase.session)
	require.NoErrorf(t, err, "failed to delete PFCP session")

	verifyNoEntries(t, testcase.expected)
}

func testUEAttachDetach(t *testing.T, testcase *TestCase) {
	testUEAttach(t, testcase)
	testUEDetach(t, testcase)
}

func testSliceMeter(t *testing.T, testcase *TestCase) {
	if isDatapathUP4() {
		err := PushSliceMeterConfig(*testcase.input.sliceConfig)
		if err != nil {
			t.Error("Error when pushing slice meter config via REST APIs", err)
		}

		verifySliceMeter(t, testcase.expected)
	} else {
		t.Skip("TODO: implement slice meter test for BESS")
	}
}
