package smf

import (
	"github.com/c-robinson/iplib"
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/test/integration"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"net"
	"time"
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
	upfAddress    string

	activeSessions int

	lastUEAddress net.IP

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, ueAddressPool string, nodeBAddress string, upfAddress string, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	pfcpClient := pfcpsim.NewPFCPClient(lAddr)

	return &MockSMF{
		log:           logger,
		ueAddressPool: ueAddressPool,
		nodeBAddress:  nodeBAddress,
		upfAddress:    upfAddress,
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

	time.Sleep(pfcpsim.HeartbeatPeriod)

	if !m.client.IsAssociationAlive() {
		m.log.Errorf("Error while peeking heartbeat response: %v", err)
		return
	}

	m.log.Infof("Setup association completed")
}

// getNextUEAddress retrieves the next available IP address from ueAddressPool
func (m *MockSMF) getNextUEAddress() net.IP {
	// TODO handle case net address is full
	if m.lastUEAddress == nil {
		// ueAddressPool is already validated
		ueIpFromPool, _, _ := net.ParseCIDR(m.ueAddressPool)
		m.lastUEAddress = iplib.NextIP(ueIpFromPool)

		return m.lastUEAddress

	} else {
		m.lastUEAddress = iplib.NextIP(m.lastUEAddress)
		return m.lastUEAddress
	}
}

// InitializeSessions create 'count' sessions, incrementally, using baseID as base to create session's rule IDs.
func (m *MockSMF) InitializeSessions(count int) {

	for i := 1; i < (count + 1); i++ {
		// using variables to ease comprehension on how rules are linked together
		uplinkTEID := uint32(i + 10)
		downlinkTEID := uint32(i + 11)

		uplinkFarID := uint32(i)
		downlinkFarID := uint32(i + 1)

		uplinkPdrID := uint16(i)
		dowlinkPdrID := uint16(i + 1)

		sessQerID := uint32(i + 3)
		appQerID := uint32(i)

		uplinkAppQerID := uint32(i)
		downlinkAppQerID := uint32(i + 1)

		pdrs := []*ie.IE{
			integration.NewUplinkPDR(integration.Create, uplinkPdrID, uplinkTEID, m.upfAddress, uplinkFarID, sessQerID, uplinkAppQerID),
			integration.NewDownlinkPDR(integration.Create, dowlinkPdrID, m.getNextUEAddress().String(), downlinkFarID, sessQerID, downlinkAppQerID),
		}

		fars := []*ie.IE{
			integration.NewUplinkFAR(integration.Create, uplinkFarID, ActionForward),
			integration.NewDownlinkFAR(integration.Create, downlinkFarID, ActionDrop, downlinkTEID, m.nodeBAddress),
		}

		qers := []*ie.IE{
			// session QER
			integration.NewQER(integration.Create, sessQerID, 0x09, 500000, 500000, 0, 0),
			// application QER
			integration.NewQER(integration.Create, appQerID, 0x08, 50000, 50000, 30000, 30000),
		}

		err := m.client.EstablishSession(pdrs, fars, qers)
		if err != nil {
			m.log.Errorf("Error while establishing sessions: %v", err)
		}

		// TODO show session's F-SEID
		m.activeSessions++
		m.log.Infof("Created sessions")
	}

}

func (m *MockSMF) DeleteAllSessions() {
	err := m.client.DeleteAllSessions()
	if err != nil {
		m.log.Errorf("Error while deleting sessions: %v", err)
		return
	}

	m.log.Infof("Deleted all sessions")
}
