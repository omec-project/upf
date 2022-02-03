// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022 Open Networking Foundation

package main

import (
	"errors"
	"fmt"
	"math"
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

// portFilter encapsulates a L4 port range as seen in PDRs. A zero value portFilter represents
// a wildcard match, but use of the dedicated new*PortFilter() functions is encouraged.
type portFilter struct {
	portLow  uint16
	portHigh uint16
}

// newWildcardPortFilter returns a portFilter that matches on every possible port, i.e., implements
// no filtering.
func newWildcardPortFilter() portFilter {
	return portFilter{
		portLow:  0,
		portHigh: math.MaxUint16,
	}
}

// newExactMatchPortFilter returns a portFilter that matches on exactly the given port.
func newExactMatchPortFilter(port uint16) portFilter {
	return portFilter{
		portLow:  port,
		portHigh: port,
	}
}

// newRangeMatchPortFilter returns a portFilter that matches on the given range [low, high].
// low must be smaller than high. Creating exact and wildcard matches with this function is
// possible, but use of the dedicated functions is encouraged.
func newRangeMatchPortFilter(low, high uint16) portFilter {
	if low > high {
		return portFilter{}
	}
	return portFilter{
		portLow:  low,
		portHigh: high,
	}
}

// newTernaryMatchPortFilter returns a portFilter that matches on the given port and mask. Only
// trivial mask are supported.
func newTernaryMatchPortFilter(port, mask uint16) (portFilter, error) {
	if mask == 0 {
		return newWildcardPortFilter(), nil
	} else if mask == math.MaxUint16 {
		return newExactMatchPortFilter(port), nil
	} else {
		return portFilter{}, ErrInvalidArgument("newTernaryMatchPortFilter", mask)
	}
}

func (pr portFilter) String() string {
	return fmt.Sprintf("{%v-%v}", pr.portLow, pr.portHigh)
}

// Width returns the number of ports covered by this portFilter.
func (pr portFilter) Width() uint16 {
	// Need to handle the zero value.
	if pr.isWildcardMatch() {
		return math.MaxUint16
	} else {
		return pr.portHigh - pr.portLow + 1
	}
}

func (pr portFilter) isWildcardMatch() bool {
	return pr.portLow == 0 && pr.portHigh == math.MaxUint16 ||
		pr.portLow == 0 && pr.portHigh == 0
}

func (pr portFilter) isExactMatch() bool {
	return pr.portLow == pr.portHigh && pr.portHigh != 0
}

func (pr portFilter) isRangeMatch() bool {
	return !pr.isExactMatch() && !pr.isWildcardMatch()
}

// Returns portFilter as an exact match, without checking if it is one. isExactMatch() must be true
// before calling asExactMatchUnchecked.
func (pr portFilter) asExactMatchUnchecked() portFilterTernaryRule {
	return portFilterTernaryRule{port: pr.portLow, mask: math.MaxUint16}
}

func (pr portFilter) asRangeMatchUnchecked() (uint16, uint16) {
	return pr.portLow, pr.portHigh
}

// Return portFilter as a trivial, single value and mask, ternary match. Will fail if conversion is
// not possible.
func (pr portFilter) asTrivialTernaryMatch() (portFilterTernaryRule, error) {
	if pr.isWildcardMatch() {
		return portFilterTernaryRule{0, 0}, nil
	} else if pr.isExactMatch() {
		return pr.asExactMatchUnchecked(), nil
	}

	return portFilterTernaryRule{}, ErrInvalidArgumentWithReason("asTrivialTernaryMatch", pr, "not trivially convertible")
}

type RangeConversionStrategy int

const (
	Exact RangeConversionStrategy = iota
	Ternary
	Hybrid
)

// Returns portFilter as a list of ternary matches that cover the same range.
func (pr portFilter) asComplexTernaryMatches(strategy RangeConversionStrategy) ([]portFilterTernaryRule, error) {
	rules := make([]portFilterTernaryRule, 0)

	// Fast path for exact and wildcard matches which are trivial.
	if pr.isExactMatch() {
		rules = append(rules, pr.asExactMatchUnchecked())
		return rules, nil
	}
	if pr.isWildcardMatch() {
		rules = append(rules, portFilterTernaryRule{0, 0})
		return rules, nil
	}

	if strategy == Exact {
		if pr.Width() > 100 {
			return nil, ErrInvalidArgumentWithReason("asComplexTernaryMatches", pr,
				"port range too wide for exact match strategy")
		}
		for port := int(pr.portLow); port <= int(pr.portHigh); port++ {
			rules = append(rules, portFilterTernaryRule{uint16(port), math.MaxUint16})
		}
	} else if strategy == Ternary {
		// Adapted from https://stackoverflow.com/a/66959276
		const limit = math.MaxUint16
		maxPort := func(port, mask uint16) uint16 {
			xid := limit - mask
			nid := port & mask
			return nid + xid
		}

		portMask := func(port, end uint16) uint16 {
			bit := uint16(1)
			mask := uint16(limit)
			testMask := uint16(limit)
			netPort := port & limit
			maximumPort := maxPort(netPort, limit)

			for netPort > 0 && maximumPort < end {
				netPort = port & testMask
				if netPort < port {
					break
				}
				maximumPort = maxPort(netPort, testMask)
				if maximumPort <= end {
					mask = testMask
				}
				testMask -= bit
				bit <<= 1
			}
			return mask
		}

		port := uint32(pr.portLow) // Promote to higher bit width for greater-equals check.
		for port <= uint32(pr.portHigh) {
			mask := portMask(uint16(port), pr.portHigh)
			rules = append(rules, portFilterTernaryRule{uint16(port), mask})
			port = uint32(maxPort(uint16(port), mask)) + 1
		}
	} else {
		return nil, ErrInvalidArgument("asComplexTernaryMatches", strategy)
	}

	return rules, nil
}

type portFilterTernaryRule struct {
	port, mask uint16
}

func (pf portFilterTernaryRule) String() string {
	return fmt.Sprintf("{0b%b & 0b%b}", pf.port, pf.mask)
}

type portFilterTernaryCartesianProduct struct {
	srcPort, srcMask uint16
	dstPort, dstMask uint16
}

// CreatePortFilterCartesianProduct converts two port ranges into a list of ternary
// rules covering the same range.
func CreatePortFilterCartesianProduct(src, dst portFilter) ([]portFilterTernaryCartesianProduct, error) {
	// A single range rule can result in multiple ternary ones. To cover the same range of packets,
	// we need to create the Cartesian product of src and dst rules. For now, we only allow one true
	// range match to keep the complexity in check.
	if src.isRangeMatch() && dst.isRangeMatch() {
		return nil, ErrInvalidArgumentWithReason("CreatePortFilterCartesianProduct",
			src, "src and dst ports cannot both be a range match")
	}

	rules := make([]portFilterTernaryCartesianProduct, 0)

	if src.isRangeMatch() {
		srcTernaryRules, err := src.asComplexTernaryMatches(Exact)
		if err != nil {
			return nil, err
		}
		dstTernary, err := dst.asTrivialTernaryMatch()
		if err != nil {
			return nil, err
		}
		for _, r := range srcTernaryRules {
			p := portFilterTernaryCartesianProduct{
				srcPort: r.port, srcMask: r.mask,
				dstPort: dstTernary.port, dstMask: dstTernary.mask,
			}
			rules = append(rules, p)
		}
	} else if dst.isRangeMatch() {
		dstTernaryRules, err := dst.asComplexTernaryMatches(Exact)
		if err != nil {
			return nil, err
		}
		srcTernary, err := src.asTrivialTernaryMatch()
		if err != nil {
			return nil, err
		}
		for _, r := range dstTernaryRules {
			p := portFilterTernaryCartesianProduct{
				srcPort: srcTernary.port, srcMask: srcTernary.mask,
				dstPort: r.port, dstMask: r.mask,
			}
			rules = append(rules, p)
		}
	} else {
		// Neither is range. Only one rule needed.
		srcTernary, err := src.asTrivialTernaryMatch()
		if err != nil {
			return nil, err
		}
		dstTernary, err := dst.asTrivialTernaryMatch()
		if err != nil {
			return nil, err
		}
		p := portFilterTernaryCartesianProduct{
			dstPort: dstTernary.port, dstMask: dstTernary.mask,
			srcPort: srcTernary.port, srcMask: srcTernary.mask,
		}
		rules = append(rules, p)
	}

	return rules, nil
}

type applicationFilter struct {
	srcIP         uint32
	dstIP         uint32
	srcPortFilter portFilter
	dstPortFilter portFilter
	proto         uint8

	srcIPMask uint32
	dstIPMask uint32
	protoMask uint8
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
	return fmt.Sprintf("ApplicationFilter(srcIP=%v/%x, dstIP=%v/%x, proto=%v/%x, srcPort=%v, dstPort=%v)",
		af.srcIP, af.srcIPMask, af.dstIP, af.dstIPMask, af.proto,
		af.protoMask, af.srcPortFilter, af.dstPortFilter)
}

func (af applicationFilter) IsEmpty() bool {
	return af.srcIP == 0 && af.dstIP == 0 && af.proto == 0 && af.srcPortFilter.isWildcardMatch() && af.dstPortFilter.isWildcardMatch()
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
					p.appFilter.dstPortFilter = ipf.dst.ports
					p.appFilter.srcPortFilter = ipf.src.ports

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
				p.appFilter.dstPortFilter = ipf.dst.ports
				p.appFilter.srcPortFilter = ipf.src.ports

				// FIXME: temporary workaround for SDF Filter,
				//  remove once we meet spec compliance
				p.appFilter.srcPortFilter = p.appFilter.dstPortFilter
				p.appFilter.dstPortFilter = newWildcardPortFilter()
			} else if p.srcIface == access {
				p.appFilter.srcIP = ip2int(ipf.dst.IPNet.IP)
				p.appFilter.srcIPMask = ipMask2int(ipf.dst.IPNet.Mask)
				p.appFilter.dstIP = ip2int(ipf.src.IPNet.IP)
				p.appFilter.dstIPMask = ipMask2int(ipf.src.IPNet.Mask)
				p.appFilter.dstPortFilter = ipf.dst.ports
				p.appFilter.srcPortFilter = ipf.src.ports

				// FIXME: temporary workaround for SDF Filter,
				//  remove once we meet spec compliance
				p.appFilter.dstPortFilter = p.appFilter.srcPortFilter
				p.appFilter.srcPortFilter = newWildcardPortFilter()
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
