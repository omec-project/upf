// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"log"
	"net"
	"strings"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func parseFlowDesc(flowDesc string) (IP4AddressNet string) {

	var err error
	var prefix string

	token := strings.Split(flowDesc, " ")
	//log.Println("IP Full String is:", token[4])
	if token[4] == "" {
		return "0.0.0.0/0"
	}
	addrString := strings.Split(token[4], "/")
	for i := range addrString {
		switch i {
		case 0:
			IP4AddressNet = addrString[i]
			if IP4AddressNet == "any" {
				return "0.0.0.0/0"
			}
		case 1:
			prefix = addrString[i]
			if err != nil {
				return IP4AddressNet + "/32"
			}
		default:
			// do nothing
		}
	}

	return IP4AddressNet + "/" + prefix
}

func parseCreatePDRPDI(pdi []*ie.IE) *pdr {

	var pdrI pdr
	for _, ie2 := range pdi {
		switch ie2.Type {
		case ie.SourceInterface:
			srcIface, err := ie2.SourceInterface()
			if err != nil {
				log.Println("Failed to parse Source Interface IE!")
				continue
			}

			if srcIface == ie.SrcInterfaceCPFunction {
				log.Println("Detected src interface cp function. Ignoring for the time being")
			} else if srcIface == ie.SrcInterfaceAccess {
				pdrI.srcIface = access
				pdrI.srcIfaceMask = 0xFF
			} else if srcIface == ie.SrcInterfaceCore {
				pdrI.srcIface = core
				pdrI.srcIfaceMask = 0xFF
			}
		case ie.FTEID:
			fteid, err := ie2.FTEID()
			if err != nil {
				log.Println("Failed to parse FTEID IE")
				continue
			}
			teid := fteid.TEID
			tunnelIPv4Address := fteid.IPv4Address

			if teid != 0 {
				pdrI.tunnelTEID = teid
				pdrI.tunnelTEIDMask = 0xFFFFFFFF
				pdrI.tunnelIP4Dst = ip2int(tunnelIPv4Address)
				pdrI.tunnelIP4DstMask = 0xFFFFFFFF
				log.Println("TunnelIPv4Address:", tunnelIPv4Address)
			}
		case ie.QFI:
			// Do nothing for the time being
		}
	}

	for _, ie2 := range pdi {
		switch ie2.Type {
		case ie.UEIPAddress:
			ueip4, err := ie2.UEIPAddress()
			if err != nil {
				log.Println("Failed to parse UE IP address")
				continue
			}

			ueIP4 := ueip4.IPv4Address
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
			if pdrI.srcIface == access {
				pdrI.srcIP = ueIP
				pdrI.srcIPMask = ueIPMask
			} else if pdrI.srcIface == core {
				pdrI.dstIP = ueIP
				pdrI.dstIPMask = ueIPMask
			}
		case ie.SDFFilter:
			// Do nothing for the time being
			sdfFields, err := ie2.SDFFilter()
			if err != nil {
				log.Println("Unable to parse SDF filter!")
				continue
			}

			flowDesc := sdfFields.FlowDescription
			if flowDesc == "" {
				log.Println("Empty SDF filter description!")
				// TODO: Implement referencing SDF ID
				continue
			}
			inetIP4Address := parseFlowDesc(flowDesc)
			log.Println("Flow Description is:", inetIP4Address)

			dstIP4, dstIP4Net, err := net.ParseCIDR(inetIP4Address)
			if err != nil {
				log.Println("Failed to parse inet IP address!, inetIP4Address:", inetIP4Address)
				// TODO: remove continue and return nil to signal error
				// once spgw-c is updated
				continue
				//return nil
			}

			var dstIP uint32
			var dstIPMask uint32

			if dstIP4.String() != "0.0.0.0" {
				dstIP = ip2int(dstIP4)
				dstIPMask = ipMask2int(dstIP4Net.Mask)
			}

			if pdrI.srcIface == access {
				pdrI.dstIP = dstIP
				pdrI.dstIPMask = dstIPMask
			} else if pdrI.srcIface == core {
				pdrI.srcIP = dstIP
				pdrI.srcIPMask = dstIPMask
			}
		}
	}

	return &pdrI
}

func parseCreatePDR(ie1 *ie.IE, fseid *ie.FSEIDFields) *pdr {

	/* reset outerHeaderRemoval to begin with */
	outerHeaderRemoval := uint8(0)

	pdrID, err := ie1.PDRID()
	if err != nil {
		log.Println("Could not read PDR ID!")
		return nil
	}

	precedence, err := ie1.Precedence()
	if err != nil {
		log.Println("Could not read Precedence!")
		return nil
	}

	pdi, err := ie1.PDI()
	if err != nil {
		log.Println("Could not read PDI!")
		return nil
	}

	res, err := ie1.OuterHeaderRemovalDescription()
	if res == 0 && err == nil { // 0 == GTP-U/UDP/IPv4
		outerHeaderRemoval = 1
	}

	// parse PDI IE and fetch srcIface, teid, and ueIPv4Address
	pdrI := parseCreatePDRPDI(pdi)
	if pdrI == nil {
		return nil
	}

	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return nil
	}

	pdrI.precedence = precedence
	pdrI.pdrID = uint32(pdrID)
	pdrI.fseID = uint32(fseid.SEID) // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
	pdrI.ctrID = 0                  // ctrID currently not being set <--- FIXIT/TODO/XXX
	pdrI.farID = uint8(farID)       // farID currently not being set <--- FIXIT/TODO/XXX
	pdrI.needDecap = outerHeaderRemoval

	// if srcIface is neither acceess nor core, then return nil
	if pdrI.srcIface != access && pdrI.srcIface != core {
		return nil
	}

	return pdrI
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
	var tunnelTEID uint32
	tunnelDst := uint32(0)
	tunnelSrc := uint32(0)
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
			tunnelTEID = outerheadercreationfields.TEID
			tunnelDst = ip2int(outerheadercreationfields.IPv4Address)
			tunnelSrc = ip2int(accessIP)
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

func parsePDRsFARs(upf *upf, sereq *message.SessionEstablishmentRequest, fseid *ie.FSEIDFields) ([]pdr, []far, error) {

	pdrs := make([]pdr, 0, MaxItems)
	fars := make([]far, 0, MaxItems)

	/* read PDR(s) */
	ies1, err := ie.ParseMultiIEs(sereq.Payload)
	if err != nil {
		return pdrs, fars, errors.New("Failed to parse sereq for IEs!")
	}

	// Iteratively go through all IEs. You can't use ie.CreatePDR or ie.CreateFAR since a single
	// message can carry multiple CreatePDR & CreateFAR messages.

	for _, ie1 := range ies1 {
		switch ie1.Type {
		case ie.CreatePDR:
			if p := parseCreatePDR(ie1, fseid); p != nil {
				pdrs = append(pdrs, *p)
			} else {
				return pdrs, fars, errors.New("Failed to parse PDR")
			}

		case ie.CreateFAR:
			if f := parseCreateFAR(ie1, fseid.SEID, upf.coreIP); f != nil {
				//printFAR(*f)
				fars = append(fars, *f)
			}
		}
	}

	return pdrs, fars, nil
}
