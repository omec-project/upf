// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"

	"github.com/wmnsk/go-pfcp/ie"
)

type far struct {
	farID uint8
	fseID uint32

	action       uint8
	tunnelType   uint8
	tunnelIP4Src uint32
	tunnelIP4Dst uint32
	tunnelTEID   uint32
	tunnelPort   uint16
}

func printFAR(far far) {
	log.Println("------------------ FAR ---------------------")
	log.Println("FAR ID:", far.farID)
	log.Println("fseID:", far.fseID)
	log.Println("action:", far.action)
	log.Println("tunnelType:", far.tunnelType)
	log.Println("tunnelIP4Src:", far.tunnelIP4Src)
	log.Println("tunnelIP4Dst:", far.tunnelIP4Dst)
	log.Println("tunnelTEID:", far.tunnelTEID)
	log.Println("tunnelPort:", far.tunnelPort)
	log.Println("--------------------------------------------")
}

func parseCreateFAR(f *ie.IE, fseid uint64, upf *upf) *far {
	return parseFAR(f, fseid, upf, "create")
}

func parseUpdateFAR(f *ie.IE, fseid uint64, upf *upf) *far {
	return parseFAR(f, fseid, upf, "update")
}

func parseFAR(f *ie.IE, fseid uint64, upf *upf, fwdType string) *far {
	farID, err := f.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return nil
	}
	// Read outerheadercreation from payload (if it exists)
	var tunnelTEID uint32
	tunnelDst := uint32(0)
	tunnelSrc := uint32(0)
	tunnelType := uint8(0)
	var fIEs []*ie.IE
	var dir uint8 = 0xFF

	if fwdType == "create" {
		fIEs, err = f.ForwardingParameters()
	} else if fwdType == "update" {
		fIEs, err = f.UpdateForwardingParameters()
	} else {
		log.Println("Invalid fwdType specified!")
		return nil
	}
	if err != nil {
		log.Println("Unable to find ForwardingParameters!")
		return nil
	}
	for _, fIE := range fIEs {
		switch fIE.Type {
		case ie.OuterHeaderCreation:
			outerheadercreationfields, err := fIE.OuterHeaderCreation()
			if err != nil {
				log.Println("Unable to parse OuterHeaderCreationFields!")
				continue
			}
			tunnelTEID = outerheadercreationfields.TEID
			tunnelDst = ip2int(outerheadercreationfields.IPv4Address)
			tunnelType = uint8(1)
		case ie.DestinationInterface:
			destinationinterface, err := fIE.DestinationInterface()
			if err != nil {
				log.Println("Unable to parse DestinationInterface field")
				continue
			}
			if destinationinterface == ie.DstInterfaceAccess {
				dir = farForwardD
				tunnelSrc = ip2int(upf.accessIP)
			} else if destinationinterface == ie.DstInterfaceCore {
				dir = farForwardU
				tunnelSrc = ip2int(upf.coreIP)
			}
		}
	}

	return &far{
		farID:        uint8(farID),  // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
		fseID:        uint32(fseid), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
		action:       dir,
		tunnelType:   tunnelType,
		tunnelIP4Src: tunnelSrc,
		tunnelIP4Dst: tunnelDst,
		tunnelTEID:   tunnelTEID,
		tunnelPort:   tunnelPort,
	}
}
