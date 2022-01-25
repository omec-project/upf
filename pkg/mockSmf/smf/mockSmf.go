package smf

import (
	"github.com/c-robinson/iplib"
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/test/integration"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"net"
)

const (
	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8
)

type MockSMF struct {
	ueAddressPool string
	nodeBAddress  string

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, ueAddressPool string, nodeBAddress string, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	pfcpClient := pfcpsim.NewPFCPClient(lAddr)

	return &MockSMF{
		log:           logger,
		ueAddressPool: ueAddressPool,
		nodeBAddress:  nodeBAddress,
		client:        pfcpClient,
	}
}

func (m *MockSMF) Disconnect() {
	m.client.DisconnectN4()
	m.log.Infof("PFCP client Disconnected")

}

func (m *MockSMF) Connect(remoteAddress string) error {
	err := m.client.ConnectN4(remoteAddress)
	if err != nil {
		return err
	}

	m.log.Infof("PFCP client is connected")
	return nil
}

func (m *MockSMF) TeardownAssociation() {
	err := m.client.TeardownAssociation()
	if err != nil {
		m.log.Errorf("Error while tearing down association: %v", err)
		return
	}

	m.log.Infoln("Teardown association completed")
}

func (m *MockSMF) SetupAssociation() {
	err := m.client.SetupAssociation()
	if err != nil {
		m.log.Errorf("Error while setting up association: %v", err)
		return
	}

	m.log.Infof("Setup association completed")

	_, err = m.client.PeekNextHeartbeatResponse(pfcpsim.Heartbeat_Period)
	if err != nil {
		m.log.Errorf("Error while peeking heartbeat response: %v", err)
		return
	}

	m.log.Infof("Received heartbeat response")
}

func (m *MockSMF) InitializeSessions(baseId int, count int, remotePeerAddress string) {
	ueIpFromPool, _, err := net.ParseCIDR(m.ueAddressPool)
	if err != nil {
		m.log.Errorf("Could not parse address pool: %v", err)
	}

	ueIpFromPool = iplib.NextIP(ueIpFromPool) // TODO handle case net address is full

	for i := baseId; i < (count + baseId); i++ {
		// using variables to ease comprehension on how sessions rules are linked together

		uplinkTeid := uint32(i)
		downlinkTeid := uint32(i + 1)

		uplinkFarID := uint32(i)
		downlinkFarID := uint32(i + 1)

		sessQerID := uint32(i + 3)

		uplinkPdrID := uint16(i)
		dowlinkPdrID := uint16(i + 1)

		uplinkAppQerID := uint32(i)
		downlinkAppQerID := uint32(i + 1)

		appQerID := uint32(i)

		upfN3Address := remotePeerAddress //FIXME is it correct?

		pdrs := []*ie.IE{
			integration.NewUplinkPDR(integration.Create, uplinkPdrID, uplinkTeid, upfN3Address, uplinkFarID, sessQerID, uplinkAppQerID),
			integration.NewDownlinkPDR(integration.Create, dowlinkPdrID, ueIpFromPool.String(), downlinkFarID, sessQerID, downlinkAppQerID),
		}

		fars := []*ie.IE{
			integration.NewUplinkFAR(integration.Create, uplinkFarID, ActionForward),
			integration.NewDownlinkFAR(integration.Create, downlinkFarID, ActionDrop, downlinkTeid, m.nodeBAddress),
		}

		qers := []*ie.IE{
			// session QER
			integration.NewQER(integration.Create, sessQerID, 0x09, 500000, 500000, 0, 0),
			// application QER
			integration.NewQER(integration.Create, appQerID, 0x08, 50000, 50000, 30000, 30000),
		}

		err = m.client.EstablishSession(pdrs, fars, qers)
		if err != nil {
			m.log.Errorf("Error while establishing sessions: %v", err)
		}

		m.log.Infof("Created sessions")
	}
}

func (m *MockSMF) DeleteAllSessions() {
	// TODO Refactor this to use new structure

}
