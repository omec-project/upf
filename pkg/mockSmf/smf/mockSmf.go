package smf

import (
	"github.com/c-robinson/iplib"
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"net"
)

const (
	INTERFACE_ACCESS = 0
	INTERFACE_CORE   = 1
)

type MockSMF struct {
	activeSessions map[uint64]pfcpsim.Session

	ueAddressPool string

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, ueAddressPool string, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	pfcpClient := pfcpsim.NewPFCPClient(lAddr)
	pfcpClient.SetLogger(logger)

	return &MockSMF{
		activeSessions: make(map[uint64]pfcpsim.Session),
		log:            logger,
		ueAddressPool:  ueAddressPool,
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
		m.log.Errorf("error while setting up association: %v", err)
	}

	m.log.Infof("setup association completed")
}

func (m *MockSMF) CreateSession(session *pfcpsim.Session) {
	err := m.client.SendSessionEstRequest(session)
	if err != nil {
		m.log.Errorf("Error while establishment of session: %v", err)
	}

	response, err := m.client.PeekNextResponse(5)
	if err != nil {
		m.log.Errorf("Error while receiving message: %v", err)
		return
	}

	if response.MessageType() == message.MsgTypeSessionEstablishmentResponse {

		for _, infoElement := range response.(*message.SessionEstablishmentResponse).IEs {
			if infoElement.Type == ie.Cause {
				cause, err := infoElement.Cause()

				if err != nil {
					m.log.Errorf("Error retrieving IE cause: %v", err)
					return
				}

				if cause != ie.CauseRequestAccepted { //FIXME should support also cause "reserved"?
					m.log.Errorf("Unexpected cause")
					return
				}
			}

			if infoElement.Type == ie.FSEID {
				// set session peerSeid
				fs, err := infoElement.FSEID()
				if err != nil {
					m.log.Errorf("Error retrieving FSEID from IE: %v", err)
					return
				}
				session.SetPeerSeid(fs.SEID)
			}
		}
	} else {
		m.log.Errorf("Received %v but was expecting %v", response.MessageType(), message.MsgTypeSessionEstablishmentResponse)
	}
}

// craftSession creates a session and saves it in ActiveSessions map
func (m *MockSMF) craftSession(baseId uint64) *pfcpsim.Session {
	seid := baseId

	if session, ok := m.activeSessions[seid]; ok {
		// Session already present. return it
		return &session
	}

	uplink := pfcpsim.UeFlow{
		Teid:  uint16(baseId),
		PdrId: uint16(baseId),
		FarId: uint16(baseId),
		QerId: uint16(baseId),
		UrrId: uint16(baseId),
	}

	downlink := pfcpsim.UeFlow{
		Teid:  uint16(baseId) + 1, //FIXME correct? uplink and downlink have different TEIDs?
		PdrId: uint16(baseId),
		FarId: uint16(baseId),
		QerId: uint16(baseId),
		UrrId: uint16(baseId),
	}

	session := pfcpsim.NewSession(nil, seid, uplink, downlink)
	m.activeSessions[seid] = *session

	m.log.Debugf("Created session with SEID %v", session.GetOurSeid())

	return session
}

func (m *MockSMF) InitializeSessions(baseId int, count int) {
	ip, _, err := net.ParseCIDR(m.ueAddressPool)
	if err != nil {
		m.log.Errorf("Could not parse address pool: %v", err)
	}

	ip = iplib.NextIP(ip) // TODO handle case net address is full

	for i := 1; i < (count + 1); i++ {
		session := m.craftSession(uint64(i))
		m.CreateSession(session)

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
			m.log.Errorf("sent session delete request but received unexpected message: %v", resp.MessageTypeName())
		}

		m.log.Infof("Session with SEID %v was successfully deleted", session.GetOurSeid())
	}
}
