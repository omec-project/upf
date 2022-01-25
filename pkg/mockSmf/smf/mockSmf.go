package smf

import (
	"github.com/c-robinson/iplib"
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/test/integration"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"net"
)

const (
	// Values for mock-up4 environment

	defaultSliceID = 0

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8
)

type MockSMF struct {
	activeSessions map[uint64]pfcpsim.Session

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
	pfcpClient.SetLogger(logger)

	return &MockSMF{
		activeSessions: make(map[uint64]pfcpsim.Session),
		log:            logger,
		ueAddressPool:  ueAddressPool,
		nodeBAddress:   nodeBAddress,
		client:         pfcpClient,
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
	}

	m.log.Infof("Setup association completed")
}

func craftUeFlow(teid uint32, pdr ie.IE, far ie.IE, qer ie.IE) (*pfcpsim.UeFlow, error) {
	// TODO still unused
	pdrId, err := pdr.PDRID()
	if err != nil {
		return nil, err
	}

	farId, err := far.FARID()
	if err != nil {
		return nil, err
	}

	qerId, err := qer.QERID()
	if err != nil {
		return nil, err
	}

	ueFlow := &pfcpsim.UeFlow{
		Teid:  teid,
		PdrId: pdrId,
		FarId: farId,
		QerId: qerId,
	}

	return ueFlow, nil
}

// craftSession creates a session using fSeid as identifier.
//If not found, a new session is created and saved in ActiveSessions map
func craftSession(fSeid uint64, ueAddress net.IP, uplinkFlow pfcpsim.UeFlow, downlinkFlow pfcpsim.UeFlow) *pfcpsim.Session {
	//if session, ok := m.activeSessions[fSeid]; ok {
	// TODO job of the caller to do this.
	//	// Session already present. return it
	//	return &session
	//}

	return pfcpsim.NewSession(fSeid, ueAddress, uplinkFlow, downlinkFlow)
}

func (m *MockSMF) InitializeSessions(baseId int, count int, remotePeerAddress string) {
	ueIpFromPool, _, err := net.ParseCIDR(m.ueAddressPool)
	if err != nil {
		m.log.Errorf("Could not parse address pool: %v", err)
	}

	ueIpFromPool = iplib.NextIP(ueIpFromPool) // TODO handle case net address is full

	for i := baseId; i < (count + 1); i++ {
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

		//m.CreateSession(session)
		uplinkUeFlow, err := craftUeFlow(uplinkTeid, *pdrs[0], *fars[0], *qers[0])
		if err != nil {
			m.log.Errorf("Error while creating ue flow: %v", err)
			return
		}

		// Interested only in session QERs // TODO handle also application QER
		downlinkUeFlow, err := craftUeFlow(downlinkTeid, *pdrs[1], *fars[1], *qers[0])
		if err != nil {
			m.log.Errorf("Error while creating downlink ue flow: %v", err)
			return
		}
		m.log.Debugf("Created uplink: %v; downlink: %v", uplinkUeFlow.Teid, downlinkUeFlow.Teid)

		err = m.client.EstablishSession(pdrs, fars, qers)
		if err != nil {
			m.log.Errorf("Error while establishing sessions: %v", err)
		}

		m.log.Infof("Created sessions")
	}
}

func (m *MockSMF) DeleteAllSessions() {
	for _, session := range m.activeSessions {
		err := m.client.SendSessionDeletionRequest(session)
		if err != nil {
			m.log.Errorf("Error while sending session deletion request: %v", err)
		}

		resp, err := m.client.PeekNextResponse(5)
		if err != nil {
			m.log.Errorf("Error while sending session deletion request: %v", err)
		}

		if resp.MessageType() != message.MsgTypeSessionDeletionResponse {
			m.log.Errorf("Sent session delete request but received unexpected message: %v", resp.MessageTypeName())
		}

		m.log.Infof("Session with SEID %v was successfully deleted", session.GetOurSeid())
	}
}
