// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
)

// CreateFAR appends far to existing list of FARs in the session
func (s *PFCPSession) CreateURR(u urr) {
	s.urrs = append(s.urrs, u)
}

// UpdateFAR updates existing far in the session
func (s *PFCPSession) UpdateURR(u urr) error {
	for idx, v := range s.urrs {
		if v.urrID == u.urrID {
			s.urrs[idx] = u
			return nil
		}
	}
	return errors.New("URR not found")
}

// RemoveFAR removes far from existing list of FARs in the session
func (s *PFCPSession) RemoveURR(id uint32) (*urr, error) {
	for idx, v := range s.urrs {
		if v.urrID == id {
			s.urrs = append(s.urrs[:idx], s.urrs[idx+1:]...)
			return &v, nil
		}
	}
	return nil, errors.New("URR not found")
}
