// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

type operation int

const (
	FwdIEOuterHeaderCreation Bits = 1 << iota
	FwdIEDestinationIntf
	FwdIEPfcpSMReqFlags
)

const (
	ActionForward = 0x2
	ActionDrop    = 0x1
	ActionBuffer  = 0x4
	ActionNotify  = 0x8
)

const (
	create operation = iota
	update
)

type far struct {
	farID   uint32
	fseID   uint64
	fseidIP uint32

	dstIntf       uint8
	sendEndMarker bool
	applyAction   uint8
	tunnelType    uint8
	tunnelIP4Src  uint32
	tunnelIP4Dst  uint32
	tunnelTEID    uint32
	tunnelPort    uint16
}

// FIXME: refactor it and use fmt.Sprintf()
func (f far) String() string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "farID: %v\n", f.farID)
	fmt.Fprintf(&b, "fseID: %x\n", f.fseID)
	fmt.Fprintf(&b, "fseIDIP: %v\n", int2ip(f.fseidIP))
	fmt.Fprintf(&b, "dstIntf: %v\n", f.dstIntf)
	fmt.Fprintf(&b, "applyAction: %v\n", f.applyAction)
	fmt.Fprintf(&b, "tunnelType: %v\n", f.tunnelType)
	fmt.Fprintf(&b, "tunnelIP4Src: %v\n", int2ip(f.tunnelIP4Src))
	fmt.Fprintf(&b, "tunnelIP4Dst: %v\n", int2ip(f.tunnelIP4Dst))
	fmt.Fprintf(&b, "tunnelTEID: %x\n", f.tunnelTEID)
	fmt.Fprintf(&b, "tunnelPort: %v\n", f.tunnelPort)
	fmt.Fprintf(&b, "sendEndMarker: %v\n", f.sendEndMarker)

	return b.String()
}

func (f *far) Drops() bool {
	if (f.applyAction & ActionDrop) != 0 {
		return true
	}

	return false
}

func (f *far) Buffers() bool {
	if (f.applyAction & ActionBuffer) != 0 {
		return true
	}

	return false
}

func (f *far) Forwards() bool {
	if (f.applyAction & ActionForward) != 0 {
		return true
	}

	return false
}

func (f *far) parseFAR(farIE *ie.IE, fseid uint64, upf *upf, op operation) error {
	f.fseID = (fseid)

	farID, err := farIE.FARID()
	if err != nil {
		return err
	}

	f.farID = farID

	action, err := farIE.ApplyAction()
	if err != nil {
		return err
	}

	if action == 0 {
		return ErrInvalidArgument("FAR Action", action)
	}

	f.applyAction = action

	var fwdIEs []*ie.IE

	switch op {
	case create:
		if (f.applyAction & ActionForward) != 0 {
			fwdIEs, err = farIE.ForwardingParameters()
		}
	case update:
		fwdIEs, err = farIE.UpdateForwardingParameters()
	default:
		return ErrInvalidOperation(op)
	}

	if err != nil {
		return err
	}

	f.sendEndMarker = false

	var fields Bits

	for _, fwdIE := range fwdIEs {
		switch fwdIE.Type {
		case ie.OuterHeaderCreation:
			fields = Set(fields, FwdIEOuterHeaderCreation)

			ohcFields, err := fwdIE.OuterHeaderCreation()
			if err != nil {
				log.Println("Unable to parse OuterHeaderCreationFields!")
				continue
			}

			f.tunnelTEID = ohcFields.TEID
			f.tunnelIP4Dst = ip2int(ohcFields.IPv4Address)
			f.tunnelType = uint8(1)  // FIXME: what does it mean?
			f.tunnelPort = tunnelGTPUPort
		case ie.DestinationInterface:
			fields = Set(fields, FwdIEDestinationIntf)

			f.dstIntf, err = fwdIE.DestinationInterface()
			if err != nil {
				log.Println("Unable to parse DestinationInterface field")
				continue
			}

			if f.dstIntf == ie.DstInterfaceAccess {
				f.tunnelIP4Src = ip2int(upf.accessIP)
			} else if f.dstIntf == ie.DstInterfaceCore {
				f.tunnelIP4Src = ip2int(upf.coreIP)
			}
		case ie.PFCPSMReqFlags:
			fields = Set(fields, FwdIEPfcpSMReqFlags)

			smReqFlags, err := fwdIE.PFCPSMReqFlags()
			if err != nil {
				log.Println("Unable to parse PFCPSMReqFlags!")
				continue
			}

			if has2ndBit(smReqFlags) {
				f.sendEndMarker = true
			}
		}
	}

	return nil
}
