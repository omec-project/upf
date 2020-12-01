// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
)

// CreateFAR appends far to existing list of FARs in the session
func (s *PFCPSession) CreateFAR(f far) {
	s.fars = append(s.fars, f)
}

// UpdateFAR updates existing far in the session
func (s *PFCPSession) UpdateFAR(f far) error {
	for idx, v := range s.fars {
		if v.farID == f.farID {
			s.fars[idx] = f
			return nil
		}
	}
	return errors.New("FAR not found")
}

// RemoveFAR removes far from existing list of FARs in the session
func (s *PFCPSession) RemoveFAR(id uint32) (*far, error) {
	for idx, v := range s.fars {
		if v.farID == id {
			s.fars = append(s.fars[:idx], s.fars[idx+1:]...)
			return &v, nil
		}
	}
	return nil, errors.New("FAR not found")
}
