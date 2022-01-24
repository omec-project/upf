package pfcpsim

import (
	"errors"
	"github.com/wmnsk/go-pfcp/ie"
	"net"
)

type UeFlow struct {
	Teid  uint16
	PdrId uint16
	FarId uint16
	QerId uint16
	UrrId uint16
}

type Session struct {
	ourSeid uint64

	ueAddress net.IP
	peerSeid  uint64

	uplink   UeFlow
	downlink UeFlow

	sentPdrs map[int]ie.IE
	sentFars map[int]ie.IE
	sentQers map[int]ie.IE
}

func NewSession(ueAddress net.IP, ourSeid uint64, uplink UeFlow, downlink UeFlow) *Session {
	return &Session{
		ourSeid:   ourSeid,
		ueAddress: ueAddress,
		peerSeid:  0, // Update later when received F-SEID IE from peer
		uplink:    uplink,
		downlink:  downlink,
		sentPdrs:  make(map[int]ie.IE),
		sentFars:  make(map[int]ie.IE),
		sentQers:  make(map[int]ie.IE),
	}
}

func (s *Session) ClearSentRules() {
	s.sentPdrs = make(map[int]ie.IE)
	s.sentFars = make(map[int]ie.IE)
	s.sentQers = make(map[int]ie.IE)
}

func CreatePDR(
	localAddress net.IP,
	ueAddress string,
	precedence uint32,
	interfaceType uint8,
	flow UeFlow,
) (*ie.IE, error) {

	if precedence == 0 {
		precedence = 2
	}

	if !(interfaceType == ie.SrcInterfaceAccess || interfaceType == ie.SrcInterfaceCore) {
		return nil, errors.New("interfaceType can be either access or core")
	}

	if ueAddress == "" {
		return nil, errors.New("ue address must be defined before creating a PDR")
	}

	teid := uint32(flow.Teid)
	pdrId := flow.PdrId
	flowDescription := "0.0.0.0/0 0.0.0.0/0 0 : 65535 0 : 65535 0x0/0x0"

	// TODO verify structure of PDR
	uplinkPDR := ie.NewCreatePDR(
		ie.NewPDRID(pdrId),
		ie.NewPrecedence(precedence),
		ie.NewPDI(
			ie.NewSourceInterface(interfaceType),
			ie.NewFTEID(0x00, teid, localAddress, nil, 0),

			ie.NewUEIPAddress(0x00, ueAddress, "", 0, 0),
			ie.NewSDFFilter(flowDescription, "", "", "", 1),
			ie.NewNetworkInstance("internetinternetinternetinterne"),
		),
	)

	farId := ie.NewCreateFAR(
		ie.NewFARID(uint32(flow.FarId)),
	)
	qerId := ie.NewCreateQER(
		ie.NewQERID(uint32(flow.QerId)),
	)

	uplinkPDR.ChildIEs = append(uplinkPDR.ChildIEs, farId, qerId)

	return uplinkPDR, nil
}

func (s *Session) IsCreated() bool {
	return s.peerSeid != 0
}

func (s *Session) GetOurSeid() uint64 {
	return s.ourSeid
}

func (s *Session) GetPeerSeid() uint64 {
	return s.peerSeid
}

func (s *Session) SetPeerSeid(peerSeid uint64) {
	s.peerSeid = peerSeid
}
