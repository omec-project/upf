// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"

	"github.com/wmnsk/go-pfcp/ie"
)

type qer struct {
	qerID    uint32
	ulStatus uint8
	dlStatus uint8
	ulMbr    uint64
	dlMbr    uint64
	ulGbr    uint64
	dlGbr    uint64
	fseID    uint32
	fseidIP  uint32
}

func (q *qer) printQER() {
	log.Println("------------------ QER ---------------------")
	log.Println("QER ID:", q.qerID)
	log.Println("fseID:", q.fseID)
	log.Println("fseIDIP:", int2ip(q.fseidIP))
	log.Println("Uplink Status:", q.ulStatus)
	log.Println("Downling Status:", q.dlStatus)
	log.Println("Uplink MBR:", q.ulMbr)
	log.Println("Downlink MBR:", q.dlMbr)
	log.Println("Uplink GBR:", q.ulGbr)
	log.Println("Downlink GBR:", q.dlGbr)
	log.Println("--------------------------------------------")
}
func (q *qer) parseQER(ie1 *ie.IE, seid uint64, upf *upf) error {

	qerID, err := ie1.QERID()
	if err != nil {
		log.Println("Could not read QER ID!")
		return nil
	}

	gsUL, err := ie1.GateStatusUL()
	if err != nil {
		log.Println("Could not read Gate status uplink!")
		return nil
	}

	gsDL, err := ie1.GateStatusDL()
	if err != nil {
		log.Println("Could not read Gate status downlink!")
		return nil
	}

	mbrUL, err := ie1.MBRUL()
	if err != nil {
		log.Println("Could not read MBRUL!")
		return nil
	}

	mbrDL, err := ie1.MBRDL()
	if err != nil {
		log.Println("Could not read MBRDL!")
		return nil
	}

	gbrUL, err := ie1.GBRUL()
	if err != nil {
		log.Println("Could not read GBRUL!")
		return nil
	}

	gbrDL, err := ie1.GBRDL()
	if err != nil {
		log.Println("Could not read GBRDL!")
		return nil
	}

	q.qerID = uint32(qerID)
	q.ulStatus = uint8(gsUL)
	q.dlStatus = uint8(gsDL)
	q.ulMbr = uint64(mbrUL)
	q.dlMbr = uint64(mbrDL)
	q.ulGbr = uint64(gbrUL)
	q.dlGbr = uint64(gbrDL)
	q.fseID = uint32(seid) // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX

	return nil
}
