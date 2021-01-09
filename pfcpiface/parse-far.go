// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"log"

	"github.com/wmnsk/go-pfcp/ie"
)

type operation int

const (
	FwdIE_OuterHeaderCreation Bits = 1 << iota
	FwdIE_DestinationIntf
)

const (
	create operation = iota
	update
)

type far struct {
	farID   uint32
	fseID   uint32
	fseidIP uint32

	action       uint8
	applyAction  uint8
	tunnelType   uint8
	tunnelIP4Src uint32
	tunnelIP4Dst uint32
	tunnelTEID   uint32
	tunnelPort   uint16
}

func (f *far) printFAR() {
	log.Println("------------------ FAR ---------------------")
	log.Println("FAR ID:", f.farID)
	log.Println("fseID:", f.fseID)
	log.Println("fseIDIP:", f.fseidIP)
	log.Println("action:", f.action)
	log.Println("applyAction:", f.applyAction)
	log.Println("tunnelType:", f.tunnelType)
	log.Println("tunnelIP4Src:", f.tunnelIP4Src)
	log.Println("tunnelIP4Dst:", f.tunnelIP4Dst)
	log.Println("tunnelTEID:", f.tunnelTEID)
	log.Println("tunnelPort:", f.tunnelPort)
	log.Println("--------------------------------------------")
}

func (f *far) parseFAR(farIE *ie.IE, fseid uint64, upf *upf, op operation) error {
	f.fseID = uint32(fseid)

	farID, err := farIE.FARID()
	if err != nil {
		return err
	}
	f.farID = farID

	action, err := farIE.ApplyAction()
	if err != nil {
		return err
	}

	if (action&0x02)>>1 == 0 {
		log.Println("Handling forward action only")
		// TODO: Handle buffer
		f.action = farDrop
		return nil
	}

	f.applyAction = action
	var fwdIEs []*ie.IE

	switch op {
	case create:
		fwdIEs, err = farIE.ForwardingParameters()
	case update:
		fwdIEs, err = farIE.UpdateForwardingParameters()
	default:
		return errors.New("Invalid op specified")
	}

	if err != nil {
		return err
	}

	var fields Bits
	for _, fwdIE := range fwdIEs {
		switch fwdIE.Type {
		case ie.OuterHeaderCreation:
			fields = Set(fields, FwdIE_OuterHeaderCreation)
			ohcFields, err := fwdIE.OuterHeaderCreation()
			if err != nil {
				log.Println("Unable to parse OuterHeaderCreationFields!")
				continue
			}
			f.tunnelTEID = ohcFields.TEID
			f.tunnelIP4Dst = ip2int(ohcFields.IPv4Address)
			f.tunnelType = uint8(1)
			f.tunnelPort = tunnelGTPUPort
		case ie.DestinationInterface:
			fields = Set(fields, FwdIE_DestinationIntf)
			dstIface, err := fwdIE.DestinationInterface()
			if err != nil {
				log.Println("Unable to parse DestinationInterface field")
				continue
			}
			if dstIface == ie.DstInterfaceAccess {
				f.action = farForwardD
				f.tunnelIP4Src = ip2int(upf.accessIP)
			} else if dstIface == ie.DstInterfaceCore {
				f.action = farForwardU
				f.tunnelIP4Src = ip2int(upf.coreIP)
			}
		}
	}

	return nil
}
