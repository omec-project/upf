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

type UeFlow struct {
	teid  uint16
	pdrId uint16
	farId uint16
	qerId uint16
	urrId uint16
}

type MockSMF struct {
	activeSessions map[uint64]Session

	ueAddressPool string

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, ueAddressPool string, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	return &MockSMF{
		activeSessions: make(map[uint64]Session),
		log:            logger,
		ueAddressPool:  ueAddressPool,
		client:         pfcpsim.NewPFCPClient(lAddr, logger),
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
		m.log.Errorf("error while tearing down association: %v", err)
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

// RecvSessionEstResponse receives messages response.
func (m *MockSMF) RecvSessionEstResponse(session *Session) {
	response, err := m.client.PeekNextResponse(5)
	if err != nil {
		m.log.Errorf("error while receiving message: %v", err)
	}

	if response.MessageType() == message.MsgTypeSessionEstablishmentResponse {

		for _, ie1 := range response.(*message.SessionEstablishmentResponse).IEs {
			if ie1.Type == ie.Cause {
				cause, err := ie1.Cause()

				if err != nil {
					m.log.Errorf("error retrieving IE cause: %v", err)
					return
				}

				if !(cause == ie.CauseRequestAccepted) { //FIXME should support also cause "reserved"?
					m.log.Errorf("unexpected cause")
					return
				}
			}

			if ie1.Type == ie.FSEID {
				// set session peerSeid
				fs, err := ie1.FSEID()
				if err != nil {
					m.log.Errorf("error retrieving FSEID from IE: %v", err)
					return
				}
				session.setPeerSeid(fs.SEID)
			}
		}
	} else {
		m.log.Errorf("received %v but was expecting %v", response.MessageType(), message.MsgTypeSessionEstablishmentResponse)
	}
}

func (m *MockSMF) CreateSession(baseId uint64) {
	seid := baseId

	if len(m.activeSessions) != 0 {
		lastSeid := m.activeSessions[uint64(len(m.activeSessions)-1)].ourSeid // get last ourSeid to generate new one
		baseId = lastSeid + 1
	}

	sess := &Session{
		ourSeid:   seid,
		ueAddress: nil,
		peerSeid:  0,

		uplink: UeFlow{
			teid:  uint16(baseId),
			pdrId: uint16(baseId),
			farId: uint16(baseId),
			qerId: uint16(baseId),
			urrId: uint16(baseId),
		},
		downlink: UeFlow{
			teid:  uint16(baseId) + 1,
			pdrId: uint16(baseId),
			farId: uint16(baseId),
			qerId: uint16(baseId),
			urrId: uint16(baseId),
		},
	}

	m.activeSessions[seid] = *sess
}

func (m *MockSMF) InitializeSessions(baseId int, count int) {
	ip, _, err := net.ParseCIDR(m.ueAddressPool)
	if err != nil {
		m.log.Errorf("could not parse address pool: %v", err)
	}

	ip = iplib.NextIP(ip) // TODO handle case net address is full

	for i := 1; i < (count + 1); i++ {
		seid := uint64(i)
		teid := uint16(i)

		ueIp := ip

		sess := Session{
			ourSeid:   seid,
			ueAddress: ueIp,
			peerSeid:  0,

			uplink: UeFlow{
				teid:  teid,
				pdrId: uint16(baseId),
				farId: uint16(baseId),
				qerId: uint16(baseId),
				urrId: uint16(baseId),
			},
			downlink: UeFlow{
				teid:  teid + 1, //FIXME correct? uplink and downlink have different TEIDs?
				pdrId: uint16(baseId),
				farId: uint16(baseId),
				qerId: uint16(baseId),
				urrId: uint16(baseId),
			},
		}

		m.log.Debugf("created session with SEID %v", sess.ourSeid)
		m.activeSessions[seid] = sess
	}
}

func craftPfcpSessionEstRequest(address net.IP, seq uint32, Seid uint64) *message.SessionEstablishmentRequest {
	IEnodeID := ie.NewNodeID(address.String(), "", "")

	fseidIE := craftFseid(address, Seid)
	msg := message.NewSessionEstablishmentRequest(
		0,
		0,
		0,
		seq,
		0,
		fseidIE,
		IEnodeID,
	)

	msg.PDNType.Type = ie.PDNType

	return msg
}

func craftFseid(address net.IP, seid uint64) *ie.IE {
	return ie.NewFSEID(seid, address, nil)
}
