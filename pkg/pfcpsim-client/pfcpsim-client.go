package pfcpsim_client

import (
	"github.com/c-robinson/iplib"
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/test/integration"
	log "github.com/sirupsen/logrus"
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

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, ueAddressPool string, nodeBAddress string, upfAddress string) *MockSMF {

	pfcpClient := pfcpsim.NewPFCPClient(lAddr)

	return &MockSMF{
		ueAddressPool: ueAddressPool,
		nodeBAddress:  nodeBAddress,
		upfAddress:    upfAddress,
		client:        pfcpClient,
	}
}

func (m *MockSMF) Disconnect() {
	m.client.DisconnectN4()
	log.Infof("PFCP client Disconnected")

}

func (m *MockSMF) Connect(remoteAddress string) error {
	err := m.client.ConnectN4(remoteAddress)
	if err != nil {
		return err
	}

	log.Infof("PFCP client is connected")
	return nil
}

// TeardownAssociation uses the PFCP client to tearing down an already established association.
// If called while no association is established by PFCP client, the latter will return an error
func (m *MockSMF) TeardownAssociation() {
	err := m.client.TeardownAssociation()
	if err != nil {
		log.Errorf("Error while tearing down association: %v", err)
		return
	}

	log.Infoln("Teardown association completed")
}

// SetupAssociation uses the PFCP client to establish an association,
// logging its success by checking PFCPclient.IsAssociationAlive
func (m *MockSMF) SetupAssociation() {
	err := m.client.SetupAssociation()
	if err != nil {
		log.Errorf("Error while setting up association: %v", err)
		return
	}

	time.Sleep(pfcpsim.DefaultHeartbeatPeriod)

	if !m.client.IsAssociationAlive() {
		log.Errorf("Error while peeking heartbeat response: %v", err)
		return
	}

	log.Infof("Setup association completed")
}

// getNextUEAddress retrieves the next available IP address from ueAddressPool
func (m *MockSMF) getNextUEAddress() net.IP {
	// TODO handle case net address is full
	if m.lastUEAddress == nil {
		ueIpFromPool, _, _ := net.ParseCIDR(m.ueAddressPool)
		m.lastUEAddress = iplib.NextIP(ueIpFromPool)

		return m.lastUEAddress

	} else {
		m.lastUEAddress = iplib.NextIP(m.lastUEAddress)
		return m.lastUEAddress
	}
}

// InitializeSessions create 'count' sessions incrementally.
// Once created, the sessions are established through PFCP client.
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
			log.Errorf("Error while establishing sessions: %v", err)
		}

		// TODO show session's F-SEID
		m.activeSessions++
		log.Infof("Created sessions")
	}

}

// DeleteAllSessions uses the PFCP client DeleteAllSessions. If failure happens at any stage,
// an error is logged through MockSMF logger.
func (m *MockSMF) DeleteAllSessions() {
	err := m.client.DeleteAllSessions()
	if err != nil {
		log.Errorf("Error while deleting sessions: %v", err)
		return
	}

	log.Infof("Deleted all sessions")
}
