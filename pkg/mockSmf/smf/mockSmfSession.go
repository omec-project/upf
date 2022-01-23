package smf

import (
	"github.com/wmnsk/go-pfcp/ie"
	"net"
)

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

func (s *Session) clearSentRules() {
	s.sentPdrs = make(map[int]ie.IE)
	s.sentFars = make(map[int]ie.IE)
	s.sentQers = make(map[int]ie.IE)
}

func (s *Session) isCreated() bool {
	return s.peerSeid != 0
}

func (s *Session) setPeerSeid(peerSeid uint64) {
	s.peerSeid = peerSeid
}
