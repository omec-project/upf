// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

type applicationFilter struct {
	srcIP   uint32
	dstIP   uint32
	srcPort uint16
	dstPort uint16
	proto   uint8

	srcIPMask   uint32
	dstIPMask   uint32
	srcPortMask uint16
	dstPortMask uint16
	protoMask   uint8
}

type pdr struct {
	srcIface     uint8
	tunnelIP4Dst uint32
	tunnelTEID   uint32
	ueAddress    uint32

	srcIfaceMask     uint8
	tunnelIP4DstMask uint32
	tunnelTEIDMask   uint32

	appFilter applicationFilter

	precedence  uint32
	pdrID       uint32
	fseID       uint64
	fseidIP     uint32
	ctrID       uint32
	farID       uint32
	qerIDList   []uint32
	needDecap   uint8
	allocIPFlag bool
}

func needAllocIP(ueIPaddr *ie.UEIPAddressFields) bool {
	if has2ndBit(ueIPaddr.Flags) && !has5thBit(ueIPaddr.Flags) {
		return false
	}

	return true
}

func (af applicationFilter) String() string {
	return fmt.Sprintf("ApplicationFilter(srcIP=%v/%x, dstIP=%v/%x, proto=%v/%x, srcPort=%v/%x, dstPort=%v/%x)",
		af.srcIP, af.srcIPMask, af.dstIP, af.dstIPMask, af.proto,
		af.protoMask, af.srcPort, af.srcPortMask, af.dstPort, af.dstPortMask)
}

func (af applicationFilter) IsEmpty() bool {
	return af.srcIP == 0 && af.dstIP == 0 && af.proto == 0 && af.srcPort == 0 && af.dstPort == 0
}

func (p pdr) String() string {
	return fmt.Sprintf("PDR(id=%v, F-SEID=%v, srcIface=%v, tunnelIPv4Dst=%v/%x, "+
		"tunnelTEID=%v/%x, ueAddress=%v, applicationFilter=%v, precedence=%v, F-SEID IP=%v, "+
		"counterID=%v, farID=%v, qerIDs=%v, needDecap=%v, allocIPFlag=%v)",
		p.pdrID, p.fseID, p.srcIface, p.tunnelIP4Dst, p.tunnelIP4DstMask,
		p.tunnelTEID, p.tunnelTEIDMask, int2ip(p.ueAddress), p.appFilter, p.precedence,
		p.fseidIP, p.ctrID, p.farID, p.qerIDList, p.needDecap, p.allocIPFlag)
}

func (p pdr) IsUplink() bool {
	return p.srcIface == access
}

func (p pdr) IsDownlink() bool {
	return p.srcIface == core
}

func (p *pdr) parsePDI(seid uint64, pdiIEs []*ie.IE, appPFDs map[string]appPFD, ippool *IPPool) error {
	var ueIP4 net.IP

	for _, pdiIE := range pdiIEs {
		switch pdiIE.Type {
		case ie.UEIPAddress:
			ueIPaddr, err := pdiIE.UEIPAddress()
			if err != nil {
				log.Warnln("Failed to parse UE IP address")
				continue
			}

			if needAllocIP(ueIPaddr) {
				/* alloc IPV6 if CHV6 is enabled : TBD */
				log.Printf("UPF should alloc UE IP for SEID %v. CHV4 flag set", seid)

				ueIP4, err = ippool.LookupOrAllocIP(seid)
				if err != nil {
					log.Errorln("failed to allocate UE IP")
					return err
				}

				log.Traceln("Found or allocated new IP", ueIP4, "from pool", ippool)

				p.allocIPFlag = true
			} else {
				ueIP4 = ueIPaddr.IPv4Address
			}
		case ie.SourceInterface:
			srcIface, err := pdiIE.SourceInterface()
			if err != nil {
				log.Println("Failed to parse Source Interface IE!")
				continue
			}

			if srcIface == ie.SrcInterfaceCPFunction {
				log.Println("Source Interface CP Function not supported yet")
			} else if srcIface == ie.SrcInterfaceAccess {
				p.srcIface = access
				p.srcIfaceMask = 0xFF
			} else if srcIface == ie.SrcInterfaceCore {
				p.srcIface = core
				p.srcIfaceMask = 0xFF
			}
		case ie.FTEID:
			fteid, err := pdiIE.FTEID()
			if err != nil {
				log.Println("Failed to parse FTEID IE")
				continue
			}

			teid := fteid.TEID
			tunnelIPv4Address := fteid.IPv4Address

			if teid != 0 {
				p.tunnelTEID = teid
				p.tunnelTEIDMask = 0xFFFFFFFF
				p.tunnelIP4Dst = ip2int(tunnelIPv4Address)
				p.tunnelIP4DstMask = 0xFFFFFFFF

				log.Println("TunnelIPv4Address:", tunnelIPv4Address)
			}
		case ie.QFI:
			// Do nothing for the time being
			continue
		}
	}

	// Needed if SDF filter is bad or absent
	if len(ueIP4) == 4 {
		p.ueAddress = ip2int(ueIP4)
	}

	for _, ie2 := range pdiIEs {
		switch ie2.Type {
		case ie.ApplicationID:
			appID, err := ie2.ApplicationID()
			if err != nil {
				log.Println("Unable to parse Application ID", err)
				continue
			}

			apfd, ok := appPFDs[appID]
			if !ok {
				log.Println("Unable to find Application ID", err)
				continue
			}

			if appID != apfd.appID {
				log.Fatalln("Mismatch in App ID", appID, apfd.appID)
			}

			log.Println("inside application id", apfd.appID, apfd.flowDescs)

			for _, flowDesc := range apfd.flowDescs {
				log.Println("flow desc", flowDesc)

				ipf, err := parseFlowDesc(flowDesc, ueIP4.String())
				if err != nil {
					return errBadFilterDesc
				}

				if (p.srcIface == access && ipf.direction == "out") || (p.srcIface == core && ipf.direction == "in") {
					log.Println("Found a match", p.srcIface, flowDesc)

					if ipf.proto != reservedProto {
						p.appFilter.proto = ipf.proto
						p.appFilter.protoMask = reservedProto
					}
					// TODO: Verify assumption that flow description in case of PFD is to be taken as-is
					p.appFilter.dstIP = ip2int(ipf.dst.IPNet.IP)
					p.appFilter.dstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
					p.appFilter.srcIP = ip2int(ipf.src.IPNet.IP)
					p.appFilter.srcIPMask = ipMask2int(ipf.src.IPNet.Mask)

					if ipf.dst.Port > 0 {
						p.appFilter.dstPort = ipf.dst.Port
						p.appFilter.dstPortMask = 0xffff
					}

					if ipf.src.Port > 0 {
						p.appFilter.srcPort = ipf.src.Port
						p.appFilter.srcPortMask = 0xffff
					}

					break
				}
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

			log.Println("Flow Description is:", flowDesc)

			ipf, err := parseFlowDesc(flowDesc, ueIP4.String())
			if err != nil {
				return errBadFilterDesc
			}

			if ipf.proto != reservedProto {
				p.appFilter.proto = ipf.proto
				p.appFilter.protoMask = reservedProto
			}

			if p.srcIface == core {
				p.appFilter.dstIP = ip2int(ipf.dst.IPNet.IP)
				p.appFilter.dstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
				p.appFilter.srcIP = ip2int(ipf.src.IPNet.IP)
				p.appFilter.srcIPMask = ipMask2int(ipf.src.IPNet.Mask)

				if ipf.src.Port > 0 {
					p.appFilter.srcPort, p.appFilter.srcPortMask = ipf.src.Port, 0xffff
				}

				if ipf.dst.Port > 0 {
					p.appFilter.dstPort, p.appFilter.dstPortMask = ipf.dst.Port, 0xffff
				}

				// FIXME: temporary workaround for SDF Filter,
				//  remove once we meet spec compliance
				p.appFilter.srcPort = p.appFilter.dstPort
				p.appFilter.dstPort, p.appFilter.dstPortMask = 0, 0 // reset UE Port
			} else if p.srcIface == access {
				p.appFilter.srcIP = ip2int(ipf.dst.IPNet.IP)
				p.appFilter.srcIPMask = ipMask2int(ipf.dst.IPNet.Mask)
				p.appFilter.dstIP = ip2int(ipf.src.IPNet.IP)
				p.appFilter.dstIPMask = ipMask2int(ipf.src.IPNet.Mask)
				if ipf.dst.Port > 0 {
					p.appFilter.srcPort, p.appFilter.srcPortMask = ipf.dst.Port, 0xffff
				}
				if ipf.src.Port > 0 {
					p.appFilter.dstPort, p.appFilter.dstPortMask = ipf.src.Port, 0xffff
				}

				// FIXME: temporary workaround for SDF Filter,
				//  remove once we meet spec compliance
				p.appFilter.dstPort = p.appFilter.srcPort
				p.appFilter.srcPort, p.appFilter.srcPortMask = 0, 0 // reset UE Port
			}
		}
	}

	return nil
}

func (p *pdr) parsePDR(ie1 *ie.IE, seid uint64, appPFDs map[string]appPFD, ippool *IPPool) error {
	/* reset outerHeaderRemoval to begin with */
	outerHeaderRemoval := uint8(0)
	p.qerIDList = make([]uint32, 0)

	pdrID, err := ie1.PDRID()
	if err != nil {
		log.Println("Could not read PDR ID!")
		return err
	}

	precedence, err := ie1.Precedence()
	if err != nil {
		log.Println("Could not read Precedence!")
		return err
	}

	pdi, err := ie1.PDI()
	if err != nil {
		log.Println("Could not read PDI!")
		return err
	}

	res, err := ie1.OuterHeaderRemovalDescription()
	if res == 0 && err == nil { // 0 == GTP-U/UDP/IPv4
		outerHeaderRemoval = 1
	}

	err = p.parsePDI(seid, pdi, appPFDs, ippool)
	if err != nil && !errors.Is(err, errBadFilterDesc) {
		return err
	}

	farID, err := ie1.FARID()
	if err != nil {
		log.Println("Could not read FAR ID!")
		return err
	}

	/* Multiple instances of QERID can be present in CreatePDR/UpdatePDR
	   go-pfcp currently support API to return list of QERIDs. So, we
	   are parsing the IE list in Application code.*/
	var ies []*ie.IE

	var errin error

	switch ie1.Type {
	case ie.CreatePDR:
		ies, errin = ie1.CreatePDR()
		if errin != nil {
			return errin
		}
	case ie.UpdatePDR:
		ies, errin = ie1.UpdatePDR()
		if errin != nil {
			return errin
		}
	}

	for _, x := range ies {
		if x.Type == ie.QERID {
			qerID, errRead := x.QERID()
			if errRead != nil {
				log.Errorln("qerID read failed")
				continue
			} else {
				p.qerIDList = append(p.qerIDList, qerID)
			}
		}
	}
	/*qerID, err := ie1.QERID()
	if err != nil {
		log.Println("Could not read QER ID!")
	}*/

	p.precedence = precedence
	p.pdrID = uint32(pdrID)
	p.fseID = (seid) // fseID currently being truncated to uint32 <--- FIXIT/TODO/XXX
	p.ctrID = 0      // ctrID currently not being set <--- FIXIT/TODO/XXX
	p.farID = farID  // farID currently not being set <--- FIXIT/TODO/XXX
	/*p.qerID = qerID*/
	p.needDecap = outerHeaderRemoval

	return nil
}
