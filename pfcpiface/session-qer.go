// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
)

// CreateQER appends qer to existing list of QERs in the session
func (s *PFCPSession) CreateQER(q qer) {
	s.qers = append(s.qers, q)
}

// UpdateQER updates existing qer in the session
func (s *PFCPSession) UpdateQER(q qer) error {
	for idx, v := range s.qers {
		if v.qerID == q.qerID {
			s.qers[idx] = q
			return nil
		}
	}
	return errors.New("QER not found")
}

// RemoveQER removes qer from existing list of QERs in the session
func (s *PFCPSession) RemoveQER(id uint32) (*qer, error) {
	for idx, v := range s.qers {
		if v.qerID == id {
			s.qers = append(s.qers[:idx], s.qers[idx+1:]...)
			return &v, nil
		}
	}
	return nil, errors.New("QER not found")
}
