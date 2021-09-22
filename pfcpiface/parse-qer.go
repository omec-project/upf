// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

type qer struct {
	qerID    uint32
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

// Satisfies the fmt.Stringer interface.
func (q qer) String() string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "qerID: %v\n", q.qerID)
	fmt.Fprintf(&b, "fseID: %x\n", q.fseID)
	fmt.Fprintf(&b, "qfi: %v\n", q.qfi)
	fmt.Fprintf(&b, "fseIDIP: %v\n", int2ip(q.fseidIP))
	fmt.Fprintf(&b, "uplinkStatus: %v\n", q.ulStatus)
	fmt.Fprintf(&b, "downlinkStatus: %v\n", q.dlStatus)
	fmt.Fprintf(&b, "uplinkMBR: %v\n", q.ulMbr)
	fmt.Fprintf(&b, "downlinkMBR: %v\n", q.dlMbr)
	fmt.Fprintf(&b, "uplinkGBR: %v\n", q.ulGbr)
	fmt.Fprintf(&b, "downlinkGBR: %v\n", q.dlGbr)

	return b.String()
}

func (q *qer) parseQER(ie1 *ie.IE, seid uint64, upf *upf) error {
	qerID, err := ie1.QERID()
	if err != nil {
		log.Println("Could not read QER ID!")
		return nil
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
