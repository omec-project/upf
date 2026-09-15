// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"github.com/omec-project/upf-epc/logger"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func addPdrInfo(msg *message.SessionEstablishmentResponse, pdrs []pdr) {
	logger.PfcpLog.Infoln("add PDRs with UPF alloc IPs to Establishment response")
	logger.PfcpLog.Infoln("PDRs:", pdrs)
	for _, pdr := range pdrs {
		logger.PfcpLog.Infoln("pdrID:", pdr.pdrID)
		if pdr.UPAllocateFteid {
			logger.PfcpLog.Infoln("adding PDR with tunnel TEID:", pdr.tunnelTEID)
			msg.CreatedPDR = append(msg.CreatedPDR,
				ie.NewCreatedPDR(
					ie.NewPDRID(uint16(pdr.pdrID)),
					ie.NewFTEID(0x01, pdr.tunnelTEID, int2ip(pdr.tunnelIP4Dst), nil, 0),
				))
		}
		if pdr.allocIPFlag && (pdr.srcIface == core) {
			logger.PfcpLog.Debugln("pdrID:", pdr.pdrID)
			var flags uint8 = 0x02
			ueIP := int2ip(pdr.ueAddress)
			logger.PfcpLog.Debugln("ueIP:", ueIP.String())
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
