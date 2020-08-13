// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func parseFlowDesc(flowDesc string) (IPAddress string, Prefix int) {

	var err error
	token := strings.Split(flowDesc, " ")
	//log.Println("IP Full String is:", token[4])
	if token[4] == "" {
		return "0.0.0.0", 0
	}
	addrString := strings.Split(token[4], "/")
	for i := range addrString {
		switch i {
		case 0:
			IPAddress = addrString[i]
			if IPAddress == "any" {
				return "0.0.0.0", 0
			}
		case 1:
			Prefix, err = strconv.Atoi(addrString[i])
			if err != nil {
				return IPAddress, 0
			}
		default:
			// do nothing
		}
	}

	return IPAddress, Prefix
}

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
			sdfFields, err := ie2.SDFFilter()
			if err != nil {
				log.Println("Unable to parse SDF filter!")
			} else {
				flowDesc := sdfFields.FlowDescription
				if flowDesc != "" {
					IPAddress, prefix := parseFlowDesc(flowDesc)
					log.Println("Flow Description is:", IPAddress, prefix)
				}
			}
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

	pdrI := pdr{
		pdrID:     uint32(pdrID),
		fseID:     uint32(fseid.SEID), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
		ctrID:     0,                  // ctrID currently not being set <--- FIXIT/TODO/XXX
		farID:     uint8(farID),       // farID currently not being set <--- FIXIT/TODO/XXX
		needDecap: outerHeaderRemoval,
	}
	// populated everything for PDR, and set the right pdr_
	if srcIface == ie.SrcInterfaceAccess {
		pdrI.srcIface = access
		pdrI.eNBTeid = teid
		pdrI.dstIP = ueIP
		pdrI.srcIfaceMask = 0xFF
		pdrI.eNBTeidMask = 0xFFFFFFFF
		pdrI.dstIPMask = ueIPMask
	} else if srcIface == ie.SrcInterfaceCore {
		pdrI.srcIface = core
		pdrI.srcIP = ueIP
		pdrI.srcIfaceMask = 0xFF
		pdrI.srcIPMask = ueIPMask
	} else {
		return nil
	}

	return &pdrI
}

func parseCreateFAR(ie1 *ie.IE, fseid uint64, coreIP net.IP) *far {
	return parseFAR(ie1, fseid, coreIP, "create")
}

func parseUpdateFAR(ie1 *ie.IE, fseid uint64, accessIP net.IP) *far {
	return parseFAR(ie1, fseid, accessIP, "update")
}

func parseFAR(ie1 *ie.IE, fseid uint64, accessIP net.IP, fwdType string) *far {
	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return nil
	}
	// Read outerheadercreation from payload (if it exists)
	var eNBTeid uint32
	eNBIP := uint32(0)
	coreIP4 := uint32(0)
	tunnelType := uint8(0)
	var ies2 []*ie.IE
	var dir uint8 = 0xFF

	if fwdType == "create" {
		ies2, err = ie1.ForwardingParameters()
	} else if fwdType == "update" {
		ies2, err = ie1.UpdateForwardingParameters()
	} else {
		log.Println("Invalid fwdType specified!")
		return nil
	}
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
			coreIP4 = ip2int(accessIP)
			tunnelType = uint8(1)
		case ie.DestinationInterface:
			destinationinterface, err := ie2.DestinationInterface()
			if err != nil {
				log.Println("Unable to parse DestinationInterface field")
				continue
			}
			if destinationinterface == ie.DstInterfaceAccess {
				dir = farForwardD
			} else if destinationinterface == ie.DstInterfaceCore {
				dir = farForwardU
			}
		}
	}

	return &far{
		farID:       uint8(farID),  // farID currently being truncated to uint8 <--- FIXIT/TODO/XXX
		fseID:       uint32(fseid), // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
		action:      dir,
		tunnelType:  tunnelType,
		accessIP:    coreIP4,
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
			if f := parseCreateFAR(ie1, fseid.SEID, upf.coreIP); f != nil {
				fars = append(fars, *f)
			}
		}
	}

	return pdrs, fars
}
