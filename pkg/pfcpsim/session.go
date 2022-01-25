package pfcpsim

import (
	"github.com/wmnsk/go-pfcp/ie"
	"net"
)

type UeFlow struct {
	Teid  uint32
	PdrId uint16
	FarId uint32
	QerId uint32
}

type Session struct {
	ourSeid uint64

	ueAddress net.IP
	peerSeid  uint64

	uplink   UeFlow
	downlink UeFlow

	sentPdrs []ie.IE
	sentFars []ie.IE
	sentQers []ie.IE
}

func NewSession(ourSeid uint64, ueAddress net.IP, uplink UeFlow, downlink UeFlow) *Session {
	return &Session{
		ourSeid:   ourSeid,
		ueAddress: ueAddress,
		peerSeid:  0, // Update later when received F-SEID IE from peer
		uplink:    uplink,
		downlink:  downlink,
		sentPdrs:  make([]ie.IE, 0),
		sentFars:  make([]ie.IE, 0),
		sentQers:  make([]ie.IE, 0),
	}
}

func (s *Session) AddPdr(pdr *ie.IE) {
	s.sentPdrs = append(s.sentPdrs, *pdr)
}

func (s *Session) AddFar(far *ie.IE) {
	s.sentFars = append(s.sentFars, *far)
}

func (s *Session) AddQer(qer *ie.IE) {
	s.sentFars = append(s.sentFars, *qer)
}

func (s *Session) ClearSentRules() {
	s.sentPdrs = make([]ie.IE, 0)
	s.sentFars = make([]ie.IE, 0)
	s.sentQers = make([]ie.IE, 0)
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
