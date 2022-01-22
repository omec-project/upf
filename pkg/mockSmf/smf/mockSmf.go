package smf

import (
	"github.com/omec-project/upf-epc/pkg/pfcpsim"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"net"
)

type Session struct {
	ourSeid uint64

	ueAddress net.IP
	peerSeid  uint64
	uplink    UeFlow
	downlink  UeFlow
	//sentPdrs  map[int]ie.IE
	//sentFars  map[int]ie.IE
	//sentQers map[int]ie.IE
}

type UeFlow struct {
	teid  uint16
	pdrId uint16
	farId uint16
	qerId uint16
	urrId uint16
}

func NewUeFlow(baseId int) *UeFlow {
	return &UeFlow{
		teid:  uint16(baseId + 1),
		pdrId: uint16(baseId + 1),
		farId: uint16(baseId + 1),
		qerId: uint16(baseId + 1),
		urrId: uint16(baseId + 1),
	}
}

type MockSMF struct {
	activeSessions map[uint64]Session

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr string, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	return &MockSMF{
		activeSessions: make(map[uint64]Session),
		log:            logger,
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

	m.log.Infoln("Teardown association finished")

}

func (m *MockSMF) SetupAssociation() {
	err := m.client.SetupAssociation()
	if err != nil {
		m.log.Errorf("error while setting up association: %v", err)
	}

	m.log.Infof("setup association completed")
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
			teid:  uint16(baseId),
			pdrId: uint16(baseId),
			farId: uint16(baseId),
			qerId: uint16(baseId),
			urrId: uint16(baseId),
		},
	}

	m.activeSessions[seid] = *sess
}

func (m *MockSMF) InitializeSessions(baseId int, count int) {
	for i := 1; i < (count + 1); i++ {
		seid := uint64(i)

		sess := Session{
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
				teid:  uint16(baseId),
				pdrId: uint16(baseId),
				farId: uint16(baseId),
				qerId: uint16(baseId),
				urrId: uint16(baseId),
			},
		}

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
