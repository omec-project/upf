// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	log "github.com/sirupsen/logrus"
)

type QosLevel uint8

const (
	ApplicationQos QosLevel = 0
	SessionQos     QosLevel = 1
)

// CreateQER appends qer to existing list of QERs in the session.
func (s *PFCPSession) CreateQER(q qer) {
	s.qers = append(s.qers, q)
}

// UpdateQER updates existing qer in the session.
func (s *PFCPSession) UpdateQER(q qer) error {
	for idx, v := range s.qers {
		if v.qerID == q.qerID {
			s.qers[idx] = q
			return nil
		}
	}

	return ErrNotFound("QER")
}

// Int version of code present at https://github.com/juliangruber/go-intersect
func Intersect(a []uint32, b []uint32) []uint32 {
	set := make([]uint32, 0)

	for i := 0; i < len(a); i++ {
		if contains(b, a[i]) {
			set = append(set, a[i])
		}
	}

	return set
}

func contains(a []uint32, val uint32) bool {
	for i := 0; i < len(a); i++ {
		if val == a[i] {
			return true
		}
	}

	return false
}

func findItemIndex(slice []uint32, val uint32) int {
	for i := 0; i < len(slice); i++ {
		if val == slice[i] {
			return i
		}
	}

	return len(slice)
}

// MarkSessionQer : identify and Mark session QER with flag.
func (s *PFCPSession) MarkSessionQer(qers []qer) {
	sessQerIDList := make([]uint32, 0)
	lastPdrIndex := len(s.pdrs) - 1
	// create search list with first pdr's qerlist */
	sessQerIDList = append(sessQerIDList, s.pdrs[lastPdrIndex].qerIDList...)

	// If PDRs have no QERs, then no marking for session qers is needed.
	// If PDRS have one QER and all PDRs point to same QER, then consider it as application qer.
	// If number of QERS is 2 or more, then search for session QER
	if (len(sessQerIDList) < 1) || (len(qers) < 2) {
		log.Infoln("need atleast 1 QER in PDR or 2 QERs in session to mark session QER.")
		return
	}

	// loop around all pdrs and find matching qers.
	for i := range s.pdrs {
		// match every qer in searchlist in pdr's qer list
		sList := Intersect(sessQerIDList, s.pdrs[i].qerIDList)
		if len(sList) == 0 {
			return
		}

		copy(sessQerIDList, sList)
	}

	// Loop through qer list and mark qer which matches
	//	  with entry in searchlist as sessionQos
	//    if len(sessQerIDList) = 1 : use as matching session QER
	//    if len(sessQerIDList) = 2 : loop and search for qer with
	//                                bigger MBR and choose as session QER
	//    if len(sessQerIDList) = 0 : no session QER
	//    if len(sessQerIDList) = 3 : TBD (UE level QER handling).
	//                                Currently handle same as len = 2
	var (
		sessionIdx int
		sessionMbr uint64
		sessQerID  uint32
	)

	if len(sessQerIDList) > 3 {
		log.Warnln("Qer ID list size above 3. Not supported.")
	}

	for idx, qer := range qers {
		if contains(sessQerIDList, qer.qerID) {
			if qer.ulGbr > 0 || qer.dlGbr > 0 {
				log.Infoln("Do not consider qer with non zero gbr value for session qer")
				continue
			}

			if qer.ulMbr >= sessionMbr {
				sessionIdx = idx
				sessQerID = qer.qerID
				sessionMbr = qer.ulMbr
			}
		}
	}

	log.Infoln("session QER found. QER ID : ", sessQerID)

	qers[sessionIdx].qosLevel = SessionQos

	for i := range s.pdrs {
		// remove common qerID from pdr's qer list
		idx := findItemIndex(s.pdrs[i].qerIDList, sessQerID)
		if idx != len(s.pdrs[i].qerIDList) {
			s.pdrs[i].qerIDList = append(s.pdrs[i].qerIDList[:idx], s.pdrs[i].qerIDList[idx+1:]...)
			s.pdrs[i].qerIDList = append(s.pdrs[i].qerIDList, sessQerID)
		}
	}
}

// RemoveQER removes qer from existing list of QERs in the session.
func (s *PFCPSession) RemoveQER(id uint32) (*qer, error) {
	for idx, v := range s.qers {
		if v.qerID == id {
			s.qers = append(s.qers[:idx], s.qers[idx+1:]...)
			return &v, nil
		}
	}

	return nil, ErrNotFound("QER")
}
