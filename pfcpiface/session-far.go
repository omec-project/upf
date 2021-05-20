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

func (s *PFCPSession) setNotifyFlag(flag bool) {
	s.notificationFlag.mux.Lock()
	defer s.notificationFlag.mux.Unlock()
	s.notificationFlag.flag = flag
}

func (s *PFCPSession) getNotifyFlag() bool {
	s.notificationFlag.mux.Lock()
	defer s.notificationFlag.mux.Unlock()
	return s.notificationFlag.flag
}

// UpdateFAR updates existing far in the session
func (s *PFCPSession) updateNotifyFlag() {
	var unset bool = true
	for _, v := range s.fars {
		if v.applyAction&ActionNotify != 0 {
			unset = false
		}
	}

	if unset {
		s.setNotifyFlag(false)
	}
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
