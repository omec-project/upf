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
	//sentPdrs  map[int]ie.IE
	//sentFars  map[int]ie.IE
	//sentQers map[int]ie.IE
}

type MockSMF struct {
	activeSessions map[int]Session

	log *logrus.Logger

	client *pfcpsim.PFCPClient
}

func NewMockSMF(lAddr net.IP, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	pfcpClient := pfcpsim.NewPFCPClient(lAddr.String())

	return &MockSMF{
		activeSessions: make(map[int]Session),
		log:            logger,
		client:         pfcpClient,
	}
}

func (m *MockSMF) Disconnect() {
	m.client.DisconnectN4()
}

func (m *MockSMF) Connect(remoteAddress string) {
	m.client.ConnectN4(remoteAddress)
}

func (m *MockSMF) createSession() {
	lastIndex := len(m.activeSessions) - 1
	lastSeid := m.activeSessions[lastIndex].ourSeid // get last ourSeid to generate new one
	newSeid := lastSeid + 1

	sess := Session{
		ourSeid:   newSeid,
		ueAddress: nil, // TODO Where to get UE-Address?
		peerSeid:  0,
	}

	m.activeSessions[lastIndex+1] = sess
}

func (m *MockSMF) InitializeSessions(count int) {
	for i := 1; i < (count + 1); i++ {
		seid := uint64(i)

		sess := Session{
			ourSeid:   seid,
			ueAddress: nil, // TODO Where to get UE-Address?
			peerSeid:  0,   // TODO Where to get peer SEID? Association Response?
		}

		m.activeSessions[i] = sess
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
