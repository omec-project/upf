// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"net"
	"testing"
	"time"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
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
			sdfFilter:    sdfFilterUDP80,
			ulTEID:       15,
			dlTEID:       16,
			QFI:          0x9,
		},
		expected: ueSessionConfig{
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

	err := pfcpClient.SendSessionEstablishmentRequest(pdrs, fars, nil, nil)
	if err != nil {
		t.Fatalf("SendSessionEstablishmentRequest failed: %v", err)
	}

	resp, err := pfcpClient.PeekNextResponse()
	if err != nil {
		t.Fatalf("PeekNextResponse failed: %v", err)
	}

	estResp, ok := resp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("Expected SessionEstablishmentResponse, got %T", resp)
	}

	testcase.expected.pdrs = pdrs
	testcase.expected.fars = fars

	remoteSEID, err := estResp.UPFSEID.FSEID()
	if err != nil {
		t.Fatalf("Failed to get FSEID: %v", err)
	}

	// the PFCP response should contain exactly 1 Create PDR IE
	if len(estResp.CreatedPDR) != 1 {
		t.Fatalf("Expected 1 CreatedPDR, got %d", len(estResp.CreatedPDR))
	}

	// verify if UE Address IE is provided and contains expected IP address
	ueIPs, err := estResp.CreatedPDR[0].UEIPAddress()
	if err != nil {
		t.Fatalf("Failed to get UE IP Address: %v", err)
	}

	expectedIP := net.ParseIP(testcase.expected.ueAddress).To4()
	actualIP := ueIPs.IPv4Address.To4()
	if !expectedIP.Equal(actualIP) {
		t.Fatalf("Expected UE IP %v, got %v", expectedIP, actualIP)
	}

	verifyEntries(t, testcase.expected)

	// no need to send modification request, we can delete PFCP session

	err = pfcpClient.SendSessionDeletionRequest(0, remoteSEID.SEID)
	if err != nil {
		t.Fatalf("SendSessionDeletionRequest failed: %v", err)
	}

	_, err = pfcpClient.PeekNextResponse()
	if err != nil {
		t.Fatalf("PeekNextResponse after deletion failed: %v", err)
	}

	verifyNoEntries(t)
}

func TestPFCPHeartbeats(t *testing.T) {
	setup(t, ConfigDefault)
	defer teardown(t)

	time.Sleep(time.Second * 10)

	// Heartbeats interval is 5 seconds by default.
	// If the association is alive after 10 seconds it means that PFCP Agent handles heartbeats properly.
	if !pfcpClient.IsAssociationAlive() {
		t.Fatal("Expected PFCP association to be alive after 10 seconds")
	}
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
				sdfFilter:    sdfFilterUDP80,
				ulTEID:       15,
				dlTEID:       16,
				QFI:          0x9,
			},
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			expected: ueSessionConfig{
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
			sdfFilter:    sdfFilterUDP80,
			ulTEID:       15,
			dlTEID:       16,
			QFI:          0x9,
		},
		expected: ueSessionConfig{
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

	sess, err := pfcpClient.EstablishSession(pdrs, fars, qers, nil)
	testcase.expected.pdrs = pdrs
	testcase.expected.fars = fars
	testcase.expected.qers = qers
	if err != nil {
		t.Fatalf("failed to establish PFCP session: %v", err)
	}
	testcase.session = sess

	verifyEntries(t, testcase.expected)

	err = pfcpClient.ModifySession(sess, nil, []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithAction(ActionForward).WithDstInterface(ie.DstInterfaceAccess).
			WithTEID(testcase.input.dlTEID).WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to modify PFCP session: %v", err)
	}

	verifyEntries(t, testcase.expected)
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

	err := pfcpClient.ModifySession(testcase.session, nil, fars, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	verifyEntries(t, testcase.expected)

	// stop buffering
	fars = []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionForward).WithTEID(testcase.input.dlTEID).
			WithDownlinkIP(testcase.input.nbAddress).BuildFAR(),
	}

	err = pfcpClient.ModifySession(testcase.session, nil, fars, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	verifyEntries(t, testcase.expected)
}

func testUEDetach(t *testing.T, testcase *testCase) {
	err := pfcpClient.DeleteSession(testcase.session)
	if err != nil {
		t.Fatalf("failed to delete PFCP session: %v", err)
	}

	verifyNoEntries(t)
}

func testUEAttachDetach(t *testing.T, testcase *testCase) {
	testUEAttach(t, testcase)
	testUEDetach(t, testcase)
}

// A session whose rule the datapath refuses must not leave its UE address allocated.
//
// Parsing allocates the address (parse_pdr.go's LookupOrAllocIP) before any rule is
// programmed, and only the deletion path releases it -- so a rejected establishment
// used to lose one address for good, against a local SEID nobody presents again.
//
// The assertion is capacity, which is the only thing that distinguishes released from
// leaked here. DeallocIP returns an address to the *back* of the free queue while
// LookupOrAllocIP takes from the front, so "the next session gets the address the
// refused one held" is false either way. With a pool of exactly two addresses, a
// refusal that releases leaves room for two more sessions; one that leaks leaves room
// for one.
//
// The refusal is arranged with an SDF filter carrying a port range 101 wide.
// CreatePortRangeCartesianProduct declines it, because asComplexTernaryMatches(Exact)
// covers a range with one ternary rule per port and refuses anything wider than 100 --
// so addPDR reports a failure before it makes an RPC, the batch completes having
// refused, and SendMsgToUPF answers rejected. That is what makes it the refusal to test
// with: it is decided client-side, so it needs no datapath fault.
//
// Both ports carry a range because parseSDFFilter's SDF workaround keeps only one of
// them, and the filter does not get to choose which. The workaround tests one fixed side
// per source interface -- the destination for core, the source for access, which that
// case has already swapped -- and when the tested side holds a range it copies that
// range onto the other side and becomes a wildcard itself, discarding whatever range the
// other side held. So the pair never reaches the datapath as a pair, and a wide range on
// each side leaves the survivor wide either way.
func TestUeIPIsReleasedWhenTheDatapathRefusesTheSession(t *testing.T) {
	setup(t, ConfigUPFBasedIPAllocationTinyPool)
	defer teardown(t)

	const aPortRangeTooWideToProgram = "permit out udp from any 100-200 to assigned 300-400"

	// The uplink PDR carries the filter the datapath will refuse; the downlink one
	// carries the CHOOSE flag that makes the UPF allocate an address.
	allocatingDownlinkPDR := func() *ie.IE {
		return ie.NewCreatePDR(
			ie.NewPDRID(2),
			ie.NewPrecedence(0),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x10, "", "", 0, 0),
				ie.NewSDFFilter(sdfFilterUDP80, "", "", "", 1),
			),
			ie.NewFARID(2),
			ie.NewQERID(2),
		)
	}

	fars := []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(1).WithDstInterface(ie.DstInterfaceCore).
			WithAction(ActionForward).BuildFAR(),
		session.NewFARBuilder().
			WithMethod(session.Create).WithID(2).
			WithDstInterface(ie.DstInterfaceAccess).
			WithAction(ActionDrop).WithTEID(16).
			WithDownlinkIP(nodeBAddress).BuildFAR(),
	}

	uplinkPDR := func(filter string) *ie.IE {
		return session.NewPDRBuilder().MarkAsUplink().
			WithMethod(session.Create).WithID(1).WithTEID(15).
			WithN3Address(upfN3Address).
			WithSDFFilter(filter).
			WithFARID(1).
			AddQERID(1).BuildPDR()
	}

	establish := func(t *testing.T, filter string) (uint8, uint64) {
		t.Helper()

		pdrs := []*ie.IE{uplinkPDR(filter), allocatingDownlinkPDR()}
		if err := pfcpClient.SendSessionEstablishmentRequest(pdrs, fars, nil, nil); err != nil {
			t.Fatalf("establishment request failed to send: %v", err)
		}

		resp, err := pfcpClient.PeekNextResponse()
		if err != nil {
			t.Fatalf("no response to the establishment: %v", err)
		}

		estResp, ok := resp.(*message.SessionEstablishmentResponse)
		if !ok {
			t.Fatalf("expected a SessionEstablishmentResponse, got %T", resp)
		}

		cause, err := estResp.Cause.Cause()
		if err != nil {
			t.Fatalf("the establishment carried no readable cause: %v", err)
		}

		// A rejected establishment carries no UP F-SEID.
		var upfSEID uint64

		if estResp.UPFSEID != nil {
			fseid, err := estResp.UPFSEID.FSEID()
			if err != nil {
				t.Fatalf("the establishment carried an unreadable UP F-SEID: %v", err)
			}

			upfSEID = fseid.SEID
		}

		return cause, upfSEID
	}

	// The refused session takes one of the two addresses and must give it back.
	if cause, _ := establish(t, aPortRangeTooWideToProgram); cause != ie.CauseRequestRejected {
		t.Fatalf("a session whose rule the datapath refused was answered with cause %d, want CauseRequestRejected (%d)",
			cause, ie.CauseRequestRejected)
	}

	// It must also leave nothing of itself behind. The refused session is the only
	// one so far, so an empty datapath means the rollback removed every rule the
	// batch had programmed before it was refused.
	verifyNoEntries(t)

	// Both remaining establishments must find an address. Without the release the
	// second has none, because the refused session is still holding one of the two.
	seids := make([]uint64, 0, 2)

	for i := 1; i <= 2; i++ {
		cause, seid := establish(t, sdfFilterUDP80)
		if cause != ie.CauseRequestAccepted {
			t.Fatalf("session %d of 2 after a refused one was answered with cause %d, want CauseRequestAccepted (%d); "+
				"the refused session did not release its address",
				i, cause, ie.CauseRequestAccepted)
		}

		seids = append(seids, seid)
	}

	for _, seid := range seids {
		if err := pfcpClient.SendSessionDeletionRequest(0, seid); err != nil {
			t.Fatalf("deletion request failed to send: %v", err)
		}

		if _, err := pfcpClient.PeekNextResponse(); err != nil {
			t.Fatalf("no response to the deletion: %v", err)
		}
	}

	verifyNoEntries(t)
}
