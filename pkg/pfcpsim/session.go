package pfcpsim

import (
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
