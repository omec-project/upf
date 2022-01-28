// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"testing"
	"time"

	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

const (
	defaultSliceID = 0

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
	// used to initialize UP4 only, then let PFCP Agent become master
	masterElectionID = p4_v1.Uint128{High: 5, Low: 0}
	// used to read table entries
	slaveElectionID = p4_v1.Uint128{High: 0, Low: 1}

	pfcpClient *pfcpsim.PFCPClient
	p4rtClient *p4rtc.Client
)

func initMockUP4() error {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", masterElectionID)
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
	err := initMockUP4()
	require.NoErrorf(t, err, "failed to initialize mock-up4")

	providers.RunDockerCommand("pfcpiface", "/bin/pfcpiface -config /config.json")

	// wait for PFCP Agent to initialize
	time.Sleep(time.Second * 3)

	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	err = pfcpClient.ConnectN4("127.0.0.1")
	require.NoErrorf(t, err, "failed to connect to UPF")

	p4rtClient, err = providers.ConnectP4rt("127.0.0.1:50001", slaveElectionID)
	require.NoErrorf(t, err, "failed to connect to P4Runtime server as slave")
}

func teardown(t *testing.T) {
	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}
	if p4rtClient != nil {
		providers.DisconnectP4rt()
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

func TestBasicSessionEstablishment(t *testing.T) {
	setup(t)
	defer teardown(t)

	err := pfcpClient.SetupAssociation()
	require.NoErrorf(t, err, "failed to setup PFCP association")

	pdrs := []*ie.IE{
		NewUplinkPDR(create, 1, 15, upfN3Address, 1, 4, 1),
		NewDownlinkPDR(create, 2, ueAddress, 2, 4, 2),
	}
	fars := []*ie.IE{
		NewUplinkFAR(create, 1, ActionForward),
		NewDownlinkFAR(create, 2, ActionDrop, 16, nodeBAddress),
	}

	qers := []*ie.IE{
		// session QER
		NewQER(create, 4, 0x09, 500000, 500000, 0, 0),
		// application QER
		NewQER(create, 1, 0x08, 50000, 50000, 30000, 30000),
	}

	err = pfcpClient.EstablishSession(pdrs, fars, qers)
	require.NoErrorf(t, err, "failed to establish PFCP session")

	// TODO: verify P4Runtime entries
}
