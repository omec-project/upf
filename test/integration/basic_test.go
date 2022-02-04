// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"testing"
	"time"

	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

const (
	defaultSliceID = 0

	defaultSDFFilter = "permit out udp from any to assigned 80-80"

	ueAddress    = "17.0.0.1"
	upfN3Address = "198.18.0.1"
	nodeBAddress = "198.18.0.10"

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8

	srcIfaceAccess = 0x1
	srcIfaceCore   = 0x2

	directionUplink   = 0x1
	directionDownlink = 0x2
)

var (
	pfcpClient *pfcpsim.PFCPClient
)

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

	testdata := &pfcpSessionData{
		nbAddress:    nodeBAddress,
		ueAddress:    ueAddress,
		upfN3Address: upfN3Address,
		sdfFilter:    defaultSDFFilter,

		ulTEID:  15,
		dlTEID:  16,
		sessQFI: 0x09,
		appQFI:  0x08,
	}

	err := pfcpClient.SetupAssociation()
	require.NoErrorf(t, err, "failed to setup PFCP association")

	pdrs := []*ie.IE{
		session.NewPDRBuilder().MarkAsUplink().
			WithMethod(session.Create).
			WithID(1).
			WithTEID(testdata.ulTEID).
			WithN3Address(testdata.upfN3Address).
			WithSDFFilter(defaultSDFFilter).
			WithFARID(1).
			AddQERID(4).
			AddQERID(1).BuildPDR(),
		session.NewPDRBuilder().MarkAsDownlink().
			WithMethod(session.Create).
			WithID(2).
			WithUEAddress(testdata.ueAddress).
			WithSDFFilter(defaultSDFFilter).
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
			WithAction(ActionDrop).WithTEID(testdata.dlTEID).WithDownlinkIP(testdata.nbAddress).BuildFAR(),
	}

	qers := []*ie.IE{
		// session QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(4).WithQFI(testdata.sessQFI).
			WithUplinkMBR(500000).
			WithDownlinkMBR(500000).
			WithUplinkGBR(0).
			WithDownlinkGBR(0).Build(),
		// uplink application QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(1).WithQFI(testdata.appQFI).
			WithUplinkMBR(50000).
			WithDownlinkMBR(50000).
			WithUplinkGBR(30000).
			WithDownlinkGBR(30000).Build(),
		// downlink application QER
		session.NewQERBuilder().WithMethod(session.Create).WithID(2).WithQFI(testdata.appQFI).
			WithUplinkMBR(50000).
			WithDownlinkMBR(50000).
			WithUplinkGBR(30000).
			WithDownlinkGBR(30000).Build(),
	}

	sess, err := pfcpClient.EstablishSession(pdrs, fars, qers)
	require.NoErrorf(t, err, "failed to establish PFCP session")

	verifyP4RuntimeEntries(t, testdata, false)

	err = pfcpClient.ModifySession(sess, nil, []*ie.IE{
		session.NewFARBuilder().
			WithMethod(session.Update).WithID(2).
			WithAction(ActionForward).WithDstInterface(ie.DstInterfaceAccess).
			WithTEID(testdata.dlTEID).WithDownlinkIP(testdata.nbAddress).BuildFAR(),
	}, nil)

	verifyP4RuntimeEntries(t, testdata, true)

	err = pfcpClient.DeleteSession(sess)
	require.NoErrorf(t, err, "failed to delete PFCP session")

	err = pfcpClient.TeardownAssociation()
	require.NoErrorf(t, err, "failed to gracefully release PFCP association")

	verifyNoP4RuntimeEntries(t)
}
