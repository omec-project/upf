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
	QerID    uint32
	QosLevel QosLevel
	Qfi      uint8
	UlStatus uint8
	DlStatus uint8
	UlMbr    uint64 // in kilobits/sec
	DlMbr    uint64 // in kilobits/sec
	UlGbr    uint64 // in kilobits/sec
	DlGbr    uint64 // in kilobits/sec
	FseID    uint64
	FseidIP  uint32
}

func (q qer) String() string {
	qosLevel, ok := qosLevelName[q.QosLevel]
	if !ok {
		qosLevel = "invalid"
	}

	return fmt.Sprintf("QER(id=%v, F-SEID=%v, F-SEID IP=%v, QFI=%v, "+
		"uplinkMBR=%v, downlinkMBR=%v, uplinkGBR=%v, downlinkGBR=%v, type=%s, "+
		"uplinkStatus=%v, downlinkStatus=%v)",
		q.QerID, q.FseID, q.FseidIP, q.Qfi, q.UlMbr, q.DlMbr, q.UlGbr, q.DlGbr,
		qosLevel, q.UlStatus, q.DlStatus)
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

	q.QerID = qerID
	q.Qfi = qfi
	q.UlStatus = gsUL
	q.DlStatus = gsDL
	q.UlMbr = mbrUL
	q.DlMbr = mbrDL
	q.UlGbr = gbrUL
	q.DlGbr = gbrDL
	q.FseID = seid

	return nil
}
