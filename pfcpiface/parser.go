// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/wmnsk/go-pfcp/ie"
)

type endpoint struct {
	IPNet *net.IPNet
	Port  uint16
}

func (ep *endpoint) parseNet(ipnet string) error {
	ipNetFields := strings.Split(ipnet, "/")
	log.Println(ipNetFields)
	switch len(ipNetFields) {
	case 1:
		ipnet = ipNetFields[0] + "/32"
	case 2:
	default:
		return errors.New("Incorrect network string")
	}

	var err error
	_, ep.IPNet, err = net.ParseCIDR(ipnet)
	if err != nil {
		return errors.New("Unable to ParseCIDR")
	}
	return nil
}

func (ep *endpoint) parsePort(port string) error {
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return err
	}
	ep.Port = uint16(p)
	return nil
}

type ipFilterRule struct {
	action, direction string
	proto             uint8
	src, dst          endpoint
}

// "permit out ip from any to assigned"
// "permit out ip from 60.60.0.102 to assigned"
// "permit out ip from 60.60.0.102/32 to assigned"
// "permit out ip from any to 60.60.0.102"
// "permit out ip from 60.60.0.1/26 to 60.60.0.102"
// "permit out ip from 60.60.0.1 8888 to 60.60.0.102/26"
// "permit out ip from 60.60.0.1 to 60.60.0.102 9999"
// "permit out ip from 60.60.0.1 8888 to 60.60.0.102 9999"

func (ipf *ipFilterRule) parseFlowDesc(flowDesc, ueIP string) error {
	fields := strings.Fields(flowDesc)

	ipf.action = fields[0]
	ipf.direction = fields[1]

	ipf.proto = parseProto(fields[2])

	// bring to common intermediate representation
	xform := func(i int) {
		log.Println(fields)
		switch fields[i] {
		case "any":
			fields[i] = "0.0.0.0/0"
		case "assigned":
			if ueIP != "" && ueIP != "<nil>" {
				fields[i] = ueIP
			} else {
				fields[i] = "0.0.0.0/0"
			}
		}
		log.Println(fields)
	}

	for i := 3; i < len(fields); i++ {
		log.Println(fields[i])
		switch fields[i] {
		case "from":
			i++
			xform(i)
			err := ipf.src.parseNet(fields[i])
			if err != nil {
				log.Println(err)
			}

			if fields[i+1] != "to" {
				i++
				ipf.src.parsePort(fields[i])
			}
		case "to":
			i++
			xform(i)
			err := ipf.dst.parseNet(fields[i])
			if err != nil {
				log.Println(err)
			}

			if i < len(fields)-1 {
				i++
				ipf.dst.parsePort(fields[i])
			}
		}
	}

	fmt.Println(ipf)
	return nil
}

func parseProto(proto string) uint8 {
	switch proto {
	case "udp":
		return 17
	case "tcp":
		return 6
	default:
		return 0xff // IANA reserved
	}
}

func parseCreatePDRPDI(pdi []*ie.IE) *pdr {
	var pdrI pdr
	var ueIP4 net.IP

	for _, ie2 := range pdi {
		switch ie2.Type {
		case ie.UEIPAddress:
			ueIPaddr, err := ie2.UEIPAddress()
			if err != nil {
				log.Println("Failed to parse UE IP address")
				continue
			}

			ueIP4 = ueIPaddr.IPv4Address
		case ie.SourceInterface:
			srcIface, err := ie2.SourceInterface()
			if err != nil {
				log.Println("Failed to parse Source Interface IE!")
				continue
			}

			if srcIface == ie.SrcInterfaceCPFunction {
				log.Println("Detected src interface cp function. Ignoring for the time being")
				return &pdrI
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
			var ipf ipFilterRule
			err = ipf.parseFlowDesc(flowDesc, ueIP4.String())
			if err != nil {
				log.Println("Failed to parse flow desc:", err)
			}
			log.Println("Flow Description is:", flowDesc, ipf)

			if ipf.proto != 0xff {
				pdrI.proto = ipf.proto
				pdrI.protoMask = 0xff
			}

			if pdrI.srcIface == access {
				pdrI.dstIP = ip2int(ipf.dst.IPNet.IP)
				pdrI.dstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
				pdrI.srcIP = ip2int(ipf.src.IPNet.IP)
				pdrI.srcIPMask = ipMask2int(ipf.src.IPNet.Mask)
			} else if pdrI.srcIface == core {
				pdrI.srcIP = ip2int(ipf.dst.IPNet.IP)
				pdrI.srcIPMask = ipMask2int(ipf.dst.IPNet.Mask)
				pdrI.dstIP = ip2int(ipf.src.IPNet.IP)
				pdrI.dstIPMask = ipMask2int(ipf.src.IPNet.Mask)
			}
		}
	}

	return &pdrI
}

func parseCreatePDR(ie1 *ie.IE, seid uint64) *pdr {
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
	pdrI.fseID = uint32(seid) // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
	pdrI.ctrID = 0            // ctrID currently not being set <--- FIXIT/TODO/XXX
	pdrI.farID = uint8(farID) // farID currently not being set <--- FIXIT/TODO/XXX
	pdrI.needDecap = outerHeaderRemoval

	// if srcIface is neither acceess nor core, then return nil
	if pdrI.srcIface != access && pdrI.srcIface != core {
		return nil
	}

	return pdrI
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
