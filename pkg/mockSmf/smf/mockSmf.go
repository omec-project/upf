package smf

import (
	"context"
	"fmt"
	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"net"
	"sync"
	"time"
)

const (
	HEARTBEAT_PERIOD   = 5 // in seconds
	PFCP_PROTOCOL_PORT = 8805
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
	seqLock        *sync.Mutex
	sequenceNumber uint32

	associationEstablished bool

	activeSessions map[int]Session

	remotePeerAddress net.IP
	localAddress      net.IP
	UdpConnection     *net.UDPConn
	recvChannel       chan message.Message
	heartbeatChannel  chan message.HeartbeatResponse

	log *logrus.Logger

	ctx              context.Context
	cancelHeartbeats context.CancelFunc
	cancelRecv       context.CancelFunc
}

func NewMockSMF(rAddr net.IP, lAddr net.IP, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	return &MockSMF{
		remotePeerAddress:      rAddr,
		localAddress:           lAddr,
		seqLock:                new(sync.Mutex),
		sequenceNumber:         0,
		associationEstablished: false,
		activeSessions:         make(map[int]Session),
		ctx:                    context.Background(),
		log:                    logger,
		recvChannel:            make(chan message.Message),
		heartbeatChannel:       make(chan message.HeartbeatResponse),
	}
}

func (m *MockSMF) Disconnect() {
	if m.cancelHeartbeats != nil {
		m.cancelHeartbeats()
	}
	if m.cancelRecv != nil {
		m.cancelRecv()
	}
	m.UdpConnection.Close()
}

func (m *MockSMF) Connect() {
	udpAddr := fmt.Sprintf("%s:%v", m.remotePeerAddress.String(), PFCP_PROTOCOL_PORT)
	m.log.Infof("Start connection to %v", udpAddr)

	rAddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}

	lAddr := &net.UDPAddr{IP: m.localAddress}
	//lAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1")} // FIXME DEBUG
	m.UdpConnection, err = net.DialUDP("udp", lAddr, rAddr)
	if err != nil {
		m.log.Fatal(err)
	}
	m.log.Debugf("Connected to %v", m.UdpConnection.RemoteAddr())

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelRecv = cancelFunc
	go m.recv(ctx)
}

func (m *MockSMF) getSequenceNumber(reset bool) uint32 {
	m.seqLock.Lock()

	m.sequenceNumber++
	if reset {
		m.sequenceNumber = 0
	}

	m.seqLock.Unlock()

	return m.sequenceNumber
}

func (m *MockSMF) sendData(data []byte) error {
	_, err := m.UdpConnection.Write(data)
	if err != nil {
		m.log.Errorf("Error while sending data to socket: %v", err)
		return err
	}

	return nil
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

func (m *MockSMF) recv(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			buffer := make([]byte, 1500)

			n, _, err := m.UdpConnection.ReadFromUDP(buffer)
			if err != nil {
				m.log.Errorf("Error on read from connection: %v", err)
				continue
			}
			msg, err := message.Parse(buffer[:n])
			if err != nil {
				m.log.Errorf("Error while parsing pfcp message: %v", err)
				continue
			}

			if hbResponse, ok := msg.(*message.HeartbeatResponse); ok {
				m.heartbeatChannel <- *hbResponse
			} else {
				m.recvChannel <- msg
			}
		}
	}
}

func (m *MockSMF) SetupPfcpAssociation() {
	assoRequest, _ := craftPfcpAssociationSetupRequest(m.remotePeerAddress, m.getSequenceNumber(true)).Marshal()

	err := m.sendData(assoRequest)
	if err != nil {
		m.log.Errorf("Error while sending association request: %v", err)
	}

	response := m.recvWithTimeout(2)

	if response.MessageType() != message.MsgTypeAssociationSetupResponse {
		m.log.Errorf("Received unexpected type of message: %v", response.MessageTypeName())
		return
	}

	log.Infof("Established PFCP association to %v", m.remotePeerAddress)
	m.associationEstablished = true

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelHeartbeats = cancelFunc
	go m.startPfcpHeartbeats(ctx)
}

func (m *MockSMF) recvWithTimeout(timeout time.Duration) message.Message {
	select {
	case msg := <-m.recvChannel:
		// TODO should I filter here? what if we intercept a message that is not the one we're interested in? e.g. heartbeat
		return msg
	case <-time.After(timeout * time.Second):
		m.log.Errorf("Timeout reached while waiting for response")
		return nil
	}
}

func (m *MockSMF) recvHeartbeatWithTimeout(timeout time.Duration) *message.HeartbeatResponse {
	select {
	case msg := <-m.heartbeatChannel:
		return &msg
	case <-time.After(timeout * time.Second):
		m.log.Errorf("Timeout reached while waiting for response")
		return nil
	}
}

func (m *MockSMF) TeardownPfcpAssociation() {
	data, err := craftPfcpAssociationReleaseRequest(m.remotePeerAddress).Marshal()
	if err != nil {
		m.log.Errorf("Error marshalling association release request: %v", err)
		return
	}

	err = m.sendData(data)
	if err != nil {
		m.log.Errorf("Error while sending data: %v", err)
	}

	response := m.recvWithTimeout(2)
	if response == nil {
		m.log.Errorf("Error while tearing down PFCP association: did not receive any message")
		return
	}
	if response.MessageType() != message.MsgTypeAssociationReleaseResponse {
		m.log.Errorf("Error while tearing down PFCP association. Received unexpected msg. Type: %v",
			response.MessageTypeName())
		return
	}

	m.cancelHeartbeats()

	// clear sessions
	m.activeSessions = make(map[int]Session)
	m.setAssociationEstablished(false)
	log.Infoln("Association removed.")

}

func (m *MockSMF) setAssociationEstablished(value bool) {
	m.associationEstablished = value
}

func (m *MockSMF) startPfcpHeartbeats(ctx context.Context) {

	period := HEARTBEAT_PERIOD * time.Second
	ticker := time.NewTicker(period)

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelHeartbeats = cancelFunc

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var localAddress net.IP = nil // FIXME recover local address
			seq := m.getSequenceNumber(false)

			hbreq := message.NewHeartbeatRequest(
				seq,
				ie.NewRecoveryTimeStamp(time.Now()),
				ie.NewSourceIPAddress(localAddress, nil, 0),
			)

			reqBytes, err := hbreq.Marshal()
			if err != nil {
				m.log.Fatalf("could not marshal heartbeat request: %v", err)
			}

			err = m.sendData(reqBytes)
			if err != nil {
				m.log.Fatalf("could not send data: %v", err)
			}

			response := m.recvHeartbeatWithTimeout(2 * HEARTBEAT_PERIOD) //FIXME
			if response == nil {
				m.log.Errorf("Did not receive any heartbeat response")
				m.setAssociationEstablished(false)
				m.cancelHeartbeats()
			}

			m.setAssociationEstablished(true)
		}
	}
}

func craftPfcpAssociationReleaseRequest(address net.IP) *message.AssociationReleaseRequest {
	ie1 := ie.NewNodeID(address.String(), "", "")

	return message.NewAssociationReleaseRequest(0, ie1)
}

func craftPfcpAssociationSetupRequest(address net.IP, seq uint32) *message.AssociationSetupRequest {
	ie1 := ie.NewNodeID(address.String(), "", "")
	ie2 := ie.NewRecoveryTimeStamp(time.Now())

	request := message.NewAssociationSetupRequest(seq, ie1, ie2)

	return request
}

func craftPfcpSessionEstRequest(address net.IP, seq uint32, Seid uint64) *message.SessionEstablishmentRequest {
	IEnodeID := ie.NewNodeID(address.String(), "", "")

	fseidIE := craftFseid(address, Seid)
	return message.NewSessionEstablishmentRequest(
		0,
		0,
		0,
		seq,
		0,
		fseidIE,
		IEnodeID)
}

func craftFseid(address net.IP, seid uint64) *ie.IE {
	return ie.NewFSEID(seid, address, nil)
}
