// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
)

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
