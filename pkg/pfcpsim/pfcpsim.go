package pfcpsim

import (
	"context"
	"errors"
	"fmt"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"net"
	"sync"
	"time"
)

const (
	PFCPStandardPort = 8805
	HEARTBEAT_PERIOD = 5 // in seconds
)

// PFCPClient enables to simulate a client sending PFCP messages towards the UPF.
// It provides two usage modes:
// - 1st mode enables high-level PFCP operations (e.g., SetupAssociation())
// - 2nd mode gives a user more control over PFCP sequence flow
//   and enables send and receive of individual messages (e.g., SendAssociationSetupRequest(), PeekNextResponse())
type PFCPClient struct {
	aliveLock          sync.Mutex
	isAssociationAlive bool

	ctx              context.Context
	cancelHeartbeats context.CancelFunc

	heartbeatsChan chan *message.HeartbeatResponse
	recvChan       chan message.Message

	sequenceNumber uint32
	seqNumLock     sync.Mutex

	localAddr string
	conn      *net.UDPConn
}

func NewPFCPClient(localAddr string) *PFCPClient {
	return &PFCPClient{
		sequenceNumber: 0,
		localAddr:      localAddr,
		ctx:            context.Background(),
		heartbeatsChan: make(chan *message.HeartbeatResponse),
		recvChan:       make(chan message.Message),
	}
}

func (c *PFCPClient) getNextSequenceNumber() uint32 {
	c.seqNumLock.Lock()
	defer c.seqNumLock.Unlock()

	c.sequenceNumber++

	return c.sequenceNumber
}

func (c *PFCPClient) resetSequenceNumber() {
	c.seqNumLock.Lock()
	defer c.seqNumLock.Unlock()

	c.sequenceNumber = 0
}

func (c *PFCPClient) setAssociationAlive(status bool) {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()

	c.isAssociationAlive = status
}

func (c *PFCPClient) sendMsg(msg message.Message) error {
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return err
	}

	if _, err := c.conn.Write(b); err != nil {
		return err
	}

	return nil
}

func (c *PFCPClient) receiveFromN4() {
	buf := make([]byte, 1500)
	for {
		n, _, err := c.conn.ReadFrom(buf)
		if err != nil {
			continue
		}

		msg, err := message.Parse(buf[:n])
		if err != nil {
			continue
		}

		if hbResp, ok := msg.(*message.HeartbeatResponse); ok {
			c.heartbeatsChan <- hbResp
		} else {
			c.recvChan <- msg
		}
	}
}

func (c *PFCPClient) ConnectN4(remoteAddr string) error {
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", remoteAddr, PFCPStandardPort))
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}

	c.conn = conn

	go c.receiveFromN4()

	return nil
}

func (c *PFCPClient) DisconnectN4() {
	if c.cancelHeartbeats != nil {
		c.cancelHeartbeats()
	}
	c.conn.Close()
}

func (c *PFCPClient) PeekNextHeartbeatResponse(timeout time.Duration) (*message.HeartbeatResponse, error) {
	select {
	case msg := <-c.heartbeatsChan:
		return msg, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for response")
	}
}

func (c *PFCPClient) PeekNextResponse(timeout time.Duration) (message.Message, error) {
	select {
	case msg := <-c.recvChan:
		return msg, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for response")
	}
}

// SendAssociationSetupRequest sends an association setup request. It allows adding custom IEs.
func (c *PFCPClient) SendAssociationSetupRequest(infoelement ...*ie.IE) error {
	c.resetSequenceNumber()

	assocReq := message.NewAssociationSetupRequest(
		c.getNextSequenceNumber(),
		ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewNodeID(c.localAddr, "", ""),
	)

	for _, ieValue := range infoelement {
		assocReq.IEs = append(assocReq.IEs, ieValue)
	}

	return c.sendMsg(assocReq)
}

func (c *PFCPClient) craftPfcpAssociationReleaseRequest(infoElement ...*ie.IE) *message.AssociationReleaseRequest {
	ie1 := ie.NewNodeID(c.conn.RemoteAddr().String(), "", "")

	c.resetSequenceNumber()
	msg := message.NewAssociationReleaseRequest(0, ie1)

	for _, ieValue := range infoElement {
		msg.IEs = append(msg.IEs, ieValue)
	}

	return msg
}

func (c *PFCPClient) SendHeartbeatRequest() error {
	hbReq := message.NewHeartbeatRequest(
		c.getNextSequenceNumber(),
		ie.NewRecoveryTimeStamp(time.Now()),
		ie.NewSourceIPAddress(net.ParseIP(c.localAddr), nil, 0),
	)

	return c.sendMsg(hbReq)
}

func (c *PFCPClient) startHeartbeats(stopCtx context.Context) {
	ticker := time.NewTicker(HEARTBEAT_PERIOD * time.Second)
	for {
		select {
		case <-stopCtx.Done():
			return
		case <-ticker.C:
			err := c.SendAndRecvHeartbeat()
			if err != nil {
				return
			}
		}
	}
}

func (c *PFCPClient) SendAndRecvHeartbeat() error {
	err := c.SendHeartbeatRequest()
	if err != nil {
		return err
	}

	_, err = c.PeekNextHeartbeatResponse(5)
	if err != nil {
		c.setAssociationAlive(false)
		return err
	}

	c.setAssociationAlive(true)

	return nil
}

func (c *PFCPClient) SetupAssociation() error {
	err := c.SendAssociationSetupRequest()
	if err != nil {
		return err
	}

	resp, err := c.PeekNextResponse(5)
	if err != nil {
		return err
	}

	if _, ok := resp.(*message.AssociationSetupResponse); !ok {
		return fmt.Errorf("invalid message received, expected association setup response")
	}

	ctx, cancelFunc := context.WithCancel(c.ctx)
	c.cancelHeartbeats = cancelFunc

	go c.startHeartbeats(ctx)

	return nil
}

func (c *PFCPClient) TeardownAssociation() error {
	msg := c.craftPfcpAssociationReleaseRequest()

	err := c.sendMsg(msg)
	if err != nil {
		return err
	}

	resp, err := c.PeekNextResponse(5)
	if err != nil {
		return err
	}

	if _, ok := resp.(*message.AssociationReleaseResponse); !ok {
		return errors.New(fmt.Sprintf("received unexpected message: %v", resp.MessageTypeName()))
	}

	if c.cancelHeartbeats != nil {
		c.cancelHeartbeats()
	}
	c.setAssociationAlive(false)

	return nil
}

func (c *PFCPClient) IsAssociationAlive() bool {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()

	return c.isAssociationAlive
}
