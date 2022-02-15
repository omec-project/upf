// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// Release allocated IPs.
func releaseAllocatedIPs(ippool *IPPool, session *PFCPSession) error {
	log.Println("release allocated IP")

	// Check if we allocated an UE IP for this session and delete it.
	for _, pdr := range session.pdrs {
		if (pdr.allocIPFlag) && (pdr.srcIface == core) {
			var ueIP net.IP = int2ip(pdr.ueAddress)

			log.Traceln("Releasing IP", ueIP, " of session", session.localSEID)

			return ippool.DeallocIP(session.localSEID)
		}
	}

	return nil
}

func addPdrInfo(msg *message.SessionEstablishmentResponse,
	session *PFCPSession) {
	log.Println("Add PDRs with UPF alloc IPs to Establishment response")

	for _, pdr := range session.pdrs {
		if (pdr.allocIPFlag) && (pdr.srcIface == core) {
			log.Println("pdrID : ", pdr.pdrID)

			var (
				flags uint8  = 0x02
				ueIP  net.IP = int2ip(pdr.ueAddress)
			)

			log.Println("ueIP : ", ueIP.String())
			msg.CreatedPDR = append(msg.CreatedPDR,
				ie.NewCreatedPDR(
					ie.NewPDRID(uint16(pdr.pdrID)),
					ie.NewUEIPAddress(flags, ueIP.String(), "", 0, 0),
				))
		}
	}
}

// CreatePDR appends pdr to existing list of PDRs in the session.
func (s *PFCPSession) CreatePDR(p pdr) {
	s.pdrs = append(s.pdrs, p)
}

// UpdatePDR updates existing pdr in the session.
func (s *PFCPSession) UpdatePDR(p pdr) error {
	for idx, v := range s.pdrs {
		if v.pdrID == p.pdrID {
			s.pdrs[idx] = p
			return nil
		}
	}

	return ErrNotFound("PDR")
}

// RemovePDR removes pdr from existing list of PDRs in the session.
func (s *PFCPSession) RemovePDR(id uint32) (*pdr, error) {
	for idx, v := range s.pdrs {
		if v.pdrID == id {
			s.pdrs = append(s.pdrs[:idx], s.pdrs[idx+1:]...)
			return &v, nil
		}
	}

	return nil, ErrNotFound("PDR")
}
