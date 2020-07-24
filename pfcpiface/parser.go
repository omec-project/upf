// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func parseCreatePDRPDI(pdi []*ie.IE) (srcIface uint8, teid uint32, ueIP4 net.IP) {
	var err error

	for _, ie2 := range pdi {
		switch ie2.Type {
		case ie.SourceInterface:
			srcIface, err = ie2.SourceInterface()
			if err != nil {
				log.Println("Failed to parse Source Interface IE!")
			} else {
				if srcIface == ie.SrcInterfaceCPFunction {
					log.Println("Detected src interface cp function. Ignoring for the time being")
				}
			}
		case ie.FTEID:
			fteid, err := ie2.FTEID()
			if err != nil {
				log.Println("Failed to parse FTEID IE")
			} else {
				teid = fteid.TEID
			}
		case ie.UEIPAddress:
			ueip4, err := ie2.UEIPAddress()
			if err != nil {
				log.Println("Failed to parse UE IP address")
			} else {
				ueIP4 = ueip4.IPv4Address
			}
		case ie.SDFFilter:
			// Do nothing for the time being
		case ie.QFI:
			// Do nothing for the time being
		}
	}
	return srcIface, teid, ueIP4
}

func parseCreatePDR(ie1 *ie.IE, fseid *ie.FSEIDFields) *pdr {
	var srcIface uint8
	var teid uint32
	var ueIP4 net.IP

	/* reset outerHeaderRemoval to begin with */
	outerHeaderRemoval := uint8(0)

	pdrID, err := ie1.PDRID()
	if err != nil {
		log.Println("Could not read PDR!")
		return nil
	}

	pdi, err := ie1.PDI()
	if err != nil {
		log.Println("Could not read PDI!")
		return nil
	}

	_, err = ie1.OuterHeaderRemoval()
	if err == nil {
		res, err := ie1.OuterHeaderRemovalDescription()
		if res == 0 && err == nil { // 0 == GTP-U/UDP/IPv4
			outerHeaderRemoval = 1
		}
	}

	// parse PDI IE and fetch srcIface, teid, and ueIPv4Address
	srcIface, teid, ueIP4 = parseCreatePDRPDI(pdi)

	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return nil
	}

	// uplink PDR may not have UE IP address IE
	// FIXIT/TODO/XXX Move this inside SrcInterfaceAccess IE check??
	var ueIP uint32
	var ueIPMask uint32
	if len(ueIP4) == 0 {
		ueIP = 0
		ueIPMask = 0
	} else {
		ueIP = ip2int(ueIP4)
		ueIPMask = 0xFFFFFFFF
	}

	// populated everything for PDR, and set the right pdr_
	if srcIface == ie.SrcInterfaceAccess {
		pdrU := pdr{
			srcIface:     access,
			eNBTeid:      teid,
			dstIP:        ueIP,
			srcIfaceMask: 0xFF,
			eNBTeidMask:  0xFFFFFFFF,
			dstIPMask:    ueIPMask,
			pdrID:        uint32(pdrID),
			fseID:        uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
			ctrID:        0,                  // ctrID currently not being set <--- FIXIT/TODO/XXX
			farID:        uint8(farID),
			needDecap:    outerHeaderRemoval,
		}
		return &pdrU
	} else if srcIface == ie.SrcInterfaceCore {
		pdrD := pdr{
			srcIface:     core,
			srcIP:        ueIP,
			srcIfaceMask: 0xFF,
			srcIPMask:    ueIPMask,
			pdrID:        uint32(pdrID),
			fseID:        uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
			ctrID:        0,                  // ctrID currently not being set <--- FIXIT/TOOD/XXX
			farID:        uint8(farID),       // farID currently not being set <--- FIXIT/TODO/XXX
			needDecap:    outerHeaderRemoval,
		}
		return &pdrD
	}

	return nil
}

func parseCreateFAR(ie1 *ie.IE, fseid *ie.FSEIDFields, n6IP net.IP) *far {
	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return nil
	}
	/* Read outerheadercreation from payload (if it exists) */
	var eNBTeid uint32
	eNBIP := uint32(0)
	n6IP4 := uint32(0)
	tunnelType := uint8(0)
	ies2, err := ie1.ForwardingParameters()
	if err != nil {
		log.Println("Unable to find ForwardingParameters!")
		return nil
	}
	for _, ie2 := range ies2 {
		switch ie2.Type {
		case ie.OuterHeaderCreation:
			outerheadercreationfields, err := ie2.OuterHeaderCreation()
			if err != nil {
				log.Println("Unable to parse OuterHeaderCreationFields!")
				continue
			}
			eNBTeid = outerheadercreationfields.TEID
			eNBIP = ip2int(outerheadercreationfields.IPv4Address)
			n6IP4 = ip2int(n6IP)
			tunnelType = uint8(1)
		}
	}

	return &far{
		farID:       uint8(farID),       // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
		fseID:       uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
		action:      farForwardU,
		tunnelType:  tunnelType,
		s1uIP:       n6IP4,
		eNBIP:       eNBIP,
		eNBTeid:     eNBTeid,
		UDPGTPUPort: udpGTPUPort,
	}
}

func parseUpdateFAR(ie1 *ie.IE, fseid uint64, n3IP net.IP) *far {
	/* check for updatefar, and fetch FAR ID */
	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Unable to find updateFAR's FAR ID!")
		return nil
	}
	var eNBTeid uint32
	eNBIP := uint32(0)
	n6IP4 := uint32(0)
	tunnelType := uint8(0)
	/* Read UpdateFAR from payload */
	ies2, err := ie1.UpdateForwardingParameters()
	if err != nil {
		log.Println("Unable to find UpdateForwardingParameters!")
		return nil
	}
	for _, ie2 := range ies2 {
		switch ie2.Type {
		case ie.OuterHeaderCreation:
			outerheadercreationfields, err := ie2.OuterHeaderCreation()
			if err != nil {
				log.Println("Unable to parse OuterHeaderCreationFields!")
			} else {
				eNBTeid = outerheadercreationfields.TEID
				eNBIP = ip2int(outerheadercreationfields.IPv4Address)
				n6IP4 = ip2int(n3IP)
				tunnelType = uint8(1)
			}
		case ie.DestinationInterface:
			// Do nothing for the time being
		}
	}

	return &far{
		farID:       uint8(farID),  // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
		fseID:       uint32(fseid), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
		action:      farForwardD,
		tunnelType:  tunnelType,
		s1uIP:       n6IP4,
		eNBIP:       eNBIP,
		eNBTeid:     eNBTeid,
		UDPGTPUPort: udpGTPUPort,
	}
}

func parsePDRFromPFCPSessEstReqPayload(upf *upf, sereq *message.SessionEstablishmentRequest, fseid *ie.FSEIDFields) ([]pdr, []far) {

	pdrs := make([]pdr, 0, MaxItems)
	fars := make([]far, 0, MaxItems)

	/* read PDR(s) */
	ies1, err := ie.ParseMultiIEs(sereq.Payload)
	if err != nil {
		log.Println("Failed to parse sereq for IEs!")
		return pdrs, fars
	}

	// Iteratively go through all IEs. You can't use ie.CreatePDR or ie.CreateFAR since a single
	// message can carry multiple CreatePDR & CreateFAR messages.

	for _, ie1 := range ies1 {
		switch ie1.Type {
		case ie.CreatePDR:
			if p := parseCreatePDR(ie1, fseid); p != nil {
				pdrs = append(pdrs, *p)
			}

		case ie.CreateFAR:
			if f := parseCreateFAR(ie1, fseid, upf.n6IP); f != nil {
				fars = append(fars, *f)
			}
		}
	}

	return pdrs, fars
}
