package integration

import (
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/stretchr/testify/require"

	"github.com/wmnsk/go-pfcp/ie"

	"testing"
	"time"
)

const (
	ueAddress    = "17.0.0.1"
	upfN3Address = "198.18.0.1"
	nodeBAddress = "198.18.0.10"

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8
)

var pfcpClient *pfcpsim.PFCPClient

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

	time.Sleep(time.Second*10)

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
