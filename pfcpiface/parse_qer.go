// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

var qosLevelName = map[QosLevel]string{
	ApplicationQos: "application",
	SessionQos:     "session",
}

type qer struct {
	qerID    uint32
	qosLevel QosLevel
	qfi      uint8
	ulStatus uint8
	dlStatus uint8
	ulMbr    uint64 // in kilobits/sec
	dlMbr    uint64 // in kilobits/sec
	ulGbr    uint64 // in kilobits/sec
	dlGbr    uint64 // in kilobits/sec
	fseID    uint64
	fseidIP  uint32
}

func (q qer) String() string {
	qosLevel, ok := qosLevelName[q.qosLevel]
	if !ok {
		qosLevel = "invalid"
	}

	return fmt.Sprintf("QER(id=%v, F-SEID=%v, F-SEID IP=%v, QFI=%v, "+
		"uplinkMBR=%v, downlinkMBR=%v, uplinkGBR=%v, downlinkGBR=%v, type=%s, "+
		"uplinkStatus=%v, downlinkStatus=%v)",
		q.qerID, q.fseID, q.fseidIP, q.qfi, q.ulMbr, q.dlMbr, q.ulGbr, q.dlGbr,
		qosLevel, q.ulStatus, q.dlStatus)
}

func (q *qer) parseQER(ie1 *ie.IE, seid uint64) error {
	qerID, err := ie1.QERID()
	if err != nil {
		log.Println("Could not read QER ID!")
		return err
	}

	qfi, err := ie1.QFI()
	if err != nil {
		log.Println("Could not read QFI!")
	}

	gsUL, err := ie1.GateStatusUL()
	if err != nil {
		log.Println("Could not read Gate status uplink!")
	}

	gsDL, err := ie1.GateStatusDL()
	if err != nil {
		log.Println("Could not read Gate status downlink!")
	}

	mbrUL, err := ie1.MBRUL()
	if err != nil {
		log.Println("Could not read MBRUL!")
	}

	mbrDL, err := ie1.MBRDL()
	if err != nil {
		log.Println("Could not read MBRDL!")
	}

	gbrUL, err := ie1.GBRUL()
	if err != nil {
		log.Println("Could not read GBRUL!")
	}

	gbrDL, err := ie1.GBRDL()
	if err != nil {
		log.Println("Could not read GBRDL!")
	}

	q.qerID = qerID
	q.qfi = qfi
	q.ulStatus = gsUL
	q.dlStatus = gsDL
	q.ulMbr = mbrUL
	q.dlMbr = mbrDL
	q.ulGbr = gbrUL
	q.dlGbr = gbrDL
	q.fseID = seid

	return nil
}
