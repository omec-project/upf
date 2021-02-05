// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"log"
	"net"
)

// Release allocated IPs
func releaseAllocatedIPs(upf *upf, session *PFCPSession) {
	log.Println("release allocated IPs")
	for _, pdr := range session.pdrs {
		if (pdr.allocIPFlag) && (pdr.srcIface == core) {
			var ueIP net.IP = int2ip(pdr.dstIP)
			log.Println("pdrID : ", pdr.pdrID, ", ueIP : ", ueIP.String())
			upf.ippool.deallocIPV4(ueIP)
		}
	}
}

func addPdrInfo(msg *message.SessionEstablishmentResponse,
	session *PFCPSession) {
	log.Println("Add PDRs with UPF alloc IPs to Establishment response")
	for _, pdr := range session.pdrs {
		if (pdr.allocIPFlag) && (pdr.srcIface == core) {
			log.Println("pdrID : ", pdr.pdrID)
			var flags uint8 = 0x02
			var ueIP net.IP = int2ip(pdr.dstIP)
			log.Println("ueIP : ", ueIP.String())
			msg.CreatedPDR = append(msg.CreatedPDR,
				ie.NewCreatedPDR(
					ie.NewPDRID(uint16(pdr.pdrID)),
					ie.NewUEIPAddress(flags, ueIP.String(), "", 0, 0),
				))
		}
	}
}

// CreatePDR appends pdr to existing list of PDRs in the session
func (s *PFCPSession) CreatePDR(p pdr) {
	s.pdrs = append(s.pdrs, p)
}

// UpdatePDR updates existing pdr in the session
func (s *PFCPSession) UpdatePDR(p pdr) error {
	for idx, v := range s.pdrs {
		if v.pdrID == p.pdrID {
			s.pdrs[idx] = p
			return nil
		}
	}
	return errors.New("PDR not found")
}

// RemovePDR removes pdr from existing list of PDRs in the session
func (s *PFCPSession) RemovePDR(id uint32) (*pdr, error) {
	for idx, v := range s.pdrs {
		if v.pdrID == id {
			s.pdrs = append(s.pdrs[:idx], s.pdrs[idx+1:]...)
			return &v, nil
		}
	}
	return nil, errors.New("PDR not found")
}
