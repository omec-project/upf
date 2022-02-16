// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"errors"
	"fmt"
	"math"
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

// portRange encapsulates a L4 port range as seen in PDRs. A zero value portRange represents
// a wildcard match, but use of the dedicated new*PortRange() functions is encouraged.
type portRange struct {
	low  uint16
	high uint16
}

// newWildcardPortRange returns a portRange that matches on every possible port, i.e., implements
// no filtering.
func newWildcardPortRange() portRange {
	return portRange{
		low:  0,
		high: math.MaxUint16,
	}
}

// newExactMatchPortRange returns a portRange that matches on exactly the given port.
func newExactMatchPortRange(port uint16) portRange {
	return portRange{
		low:  port,
		high: port,
	}
}

// newRangeMatchPortRange returns a portRange that matches on the given range [low, high].
// low must be smaller than high. Creating exact and wildcard matches with this function is
// possible, but use of the dedicated functions is encouraged.
func newRangeMatchPortRange(low, high uint16) portRange {
	if low > high {
		return portRange{}
	}

	return portRange{
		low:  low,
		high: high,
	}
}

func (pr portRange) String() string {
	return fmt.Sprintf("{%v-%v}", pr.low, pr.high)
}

// Width returns the number of ports covered by this portRange.
func (pr portRange) Width() uint16 {
	// Need to handle the zero value.
	if pr.isWildcardMatch() {
		return math.MaxUint16
	} else {
		return pr.high - pr.low + 1
	}
}

func (pr portRange) isWildcardMatch() bool {
	return pr.low == 0 && pr.high == math.MaxUint16 ||
		pr.low == 0 && pr.high == 0
}

func (pr portRange) isExactMatch() bool {
	return pr.low == pr.high && pr.high != 0
}

func (pr portRange) isRangeMatch() bool {
	return !pr.isExactMatch() && !pr.isWildcardMatch()
}

// Returns portRange as an exact match, without checking if it is one. isExactMatch() must be true
// before calling asExactMatchUnchecked.
func (pr portRange) asExactMatchUnchecked() portRangeTernaryRule {
	return portRangeTernaryRule{port: pr.low, mask: math.MaxUint16}
}

// Return portRange as a trivial, single value and mask, ternary match. Will fail if conversion is
// not possible.
func (pr portRange) asTrivialTernaryMatch() (portRangeTernaryRule, error) {
	if pr.isWildcardMatch() {
		return portRangeTernaryRule{0, 0}, nil
	} else if pr.isExactMatch() {
		return pr.asExactMatchUnchecked(), nil
	}

	return portRangeTernaryRule{}, ErrInvalidArgumentWithReason("asTrivialTernaryMatch", pr, "not trivially convertible")
}

type RangeConversionStrategy int

const (
	Exact RangeConversionStrategy = iota
	Ternary
)

// Returns portRange as a list of ternary matches that cover the same range.
func (pr portRange) asComplexTernaryMatches(strategy RangeConversionStrategy) ([]portRangeTernaryRule, error) {
	rules := make([]portRangeTernaryRule, 0)

	// Fast path for exact and wildcard matches which are trivial.
	if pr.isExactMatch() {
		rules = append(rules, pr.asExactMatchUnchecked())
		return rules, nil
	}

	if pr.isWildcardMatch() {
		rules = append(rules, portRangeTernaryRule{0, 0})
		return rules, nil
	}

	if strategy == Exact {
		if pr.Width() > 100 {
			return nil, ErrInvalidArgumentWithReason("asComplexTernaryMatches", pr,
				"port range too wide for exact match strategy")
		}

		for port := int(pr.low); port <= int(pr.high); port++ {
			rules = append(rules, portRangeTernaryRule{uint16(port), math.MaxUint16})
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

		port := uint32(pr.low) // Promote to higher bit width for greater-equals check.
		for port <= uint32(pr.high) {
			mask := portMask(uint16(port), pr.high)
			rules = append(rules, portRangeTernaryRule{uint16(port), mask})
			port = uint32(maxPort(uint16(port), mask)) + 1
		}
	} else {
		return nil, ErrInvalidArgument("asComplexTernaryMatches", strategy)
	}

	return rules, nil
}

type portRangeTernaryRule struct {
	port, mask uint16
}

func (pf portRangeTernaryRule) String() string {
	return fmt.Sprintf("{0b%b & 0b%b}", pf.port, pf.mask)
}

type portRangeTernaryCartesianProduct struct {
	srcPort, srcMask uint16
	dstPort, dstMask uint16
}

// CreatePortRangeCartesianProduct converts two port ranges into a list of ternary
// rules covering the same range.
func CreatePortRangeCartesianProduct(src, dst portRange) ([]portRangeTernaryCartesianProduct, error) {
	// A single range rule can result in multiple ternary ones. To cover the same range of packets,
	// we need to create the Cartesian product of src and dst rules. For now, we only allow one true
	// range match to keep the complexity in check.
	if src.isRangeMatch() && dst.isRangeMatch() {
		return nil, ErrInvalidArgumentWithReason("CreatePortRangeCartesianProduct",
			src, "src and dst ports cannot both be a range match")
	}

	rules := make([]portRangeTernaryCartesianProduct, 0)

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
			p := portRangeTernaryCartesianProduct{
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
			p := portRangeTernaryCartesianProduct{
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

		p := portRangeTernaryCartesianProduct{
			dstPort: dstTernary.port, dstMask: dstTernary.mask,
			srcPort: srcTernary.port, srcMask: srcTernary.mask,
		}
		rules = append(rules, p)
	}

	return rules, nil
}

type applicationFilter struct {
	srcIP        uint32
	dstIP        uint32
	srcPortRange portRange
	dstPortRange portRange
	proto        uint8

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
		int2ip(af.srcIP), af.srcIPMask, int2ip(af.dstIP), af.dstIPMask, af.proto,
		af.protoMask, af.srcPortRange, af.dstPortRange)
}

func (p pdr) String() string {
	return fmt.Sprintf("PDR(id=%v, F-SEID=%v, srcIface=%v, tunnelIPv4Dst=%v/%x, "+
		"tunnelTEID=%v/%x, ueAddress=%v, applicationFilter=%v, precedence=%v, F-SEID IP=%v, "+
		"counterID=%v, farID=%v, qerIDs=%v, needDecap=%v, allocIPFlag=%v)",
		p.pdrID, p.fseID, p.srcIface, int2ip(p.tunnelIP4Dst), p.tunnelIP4DstMask,
		p.tunnelTEID, p.tunnelTEIDMask, int2ip(p.ueAddress), p.appFilter, p.precedence,
		p.fseidIP, p.ctrID, p.farID, p.qerIDList, p.needDecap, p.allocIPFlag)
}

func (p pdr) IsAppFilterEmpty() bool {
	return p.appFilter.proto == 0 &&
		((p.IsUplink() && p.appFilter.dstIP == 0 && p.appFilter.dstPortRange.isWildcardMatch()) ||
			(p.IsDownlink() && p.appFilter.srcIP == 0 && p.appFilter.srcPortRange.isWildcardMatch()))
}

func (p pdr) IsUplink() bool {
	return p.srcIface == access
}

func (p pdr) IsDownlink() bool {
	return p.srcIface == core
}

func (p *pdr) parseUEAddressIE(ueAddrIE *ie.IE, ippool *IPPool) error {
	var ueIP4 net.IP

	ueIPaddr, err := ueAddrIE.UEIPAddress()
	if err != nil {
		return err
	}

	if needAllocIP(ueIPaddr) {
		/* alloc IPV6 if CHV6 is enabled : TBD */
		log.Infof("UPF should alloc UE IP for SEID %v. CHV4 flag set", p.fseID)

		ueIP4, err = ippool.LookupOrAllocIP(p.fseID)
		if err != nil {
			log.Errorln("failed to allocate UE IP")
			return err
		}

		log.Traceln("Found or allocated new IP", ueIP4, "from pool", ippool)

		p.allocIPFlag = true
	} else {
		ueIP4 = ueIPaddr.IPv4Address
	}

	// Needed if SDF filter is bad or absent
	if len(ueIP4) != 4 {
		return ErrOperationFailedWithParam("parse UE Address IE",
			"IP address length", len(ueIP4))
	}

	p.ueAddress = ip2int(ueIP4)

	return nil
}

func (p *pdr) parseSourceInterfaceIE(srcIfaceIE *ie.IE) error {
	srcIface, err := srcIfaceIE.SourceInterface()
	if err != nil {
		return err
	}

	if srcIface == ie.SrcInterfaceCPFunction {
		return ErrUnsupported("Source Interface CP Function", srcIface)
	} else if srcIface == ie.SrcInterfaceAccess {
		p.srcIface = access
		p.srcIfaceMask = 0xFF
	} else if srcIface == ie.SrcInterfaceCore {
		p.srcIface = core
		p.srcIfaceMask = 0xFF
	}

	return nil
}

func (p *pdr) parseFTEID(teidIE *ie.IE) error {
	fteid, err := teidIE.FTEID()
	if err != nil {
		return err
	}

	teid := fteid.TEID
	tunnelIPv4Address := fteid.IPv4Address

	if teid != 0 {
		p.tunnelTEID = teid
		p.tunnelTEIDMask = 0xFFFFFFFF
		p.tunnelIP4Dst = ip2int(tunnelIPv4Address)
		p.tunnelIP4DstMask = 0xFFFFFFFF
	}

	return nil
}

func (p *pdr) parseApplicationID(ie *ie.IE, appPFDs map[string]appPFD) error {
	appID, err := ie.ApplicationID()
	if err != nil {
		return err
	}

	apfd, ok := appPFDs[appID]
	if !ok {
		return ErrNotFoundWithParam("Application PFD for ApplicationID", "application ID", appID)
	}

	if appID != apfd.appID {
		log.Fatalln("Mismatch in App ID", appID, apfd.appID)
	}

	for _, flowDesc := range apfd.flowDescs {
		logger := log.WithFields(log.Fields{
			"Application ID":   apfd.appID,
			"Flow Description": flowDesc,
		})
		logger.Debug("Parsing flow description of Application ID IE")

		ipf, err := parseFlowDesc(flowDesc, int2ip(p.ueAddress).String())
		if err != nil {
			return errBadFilterDesc
		}

		if (p.srcIface == access && ipf.direction == "out") ||
			(p.srcIface == core && ipf.direction == "in") {
			logger.Debug("Found a matching flow description")

			if ipf.proto != reservedProto {
				p.appFilter.proto = ipf.proto
				p.appFilter.protoMask = math.MaxUint8
			}
			// TODO: Verify assumption that flow description in case of PFD is to be taken as-is
			p.appFilter.dstIP = ip2int(ipf.dst.IPNet.IP)
			p.appFilter.dstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
			p.appFilter.srcIP = ip2int(ipf.src.IPNet.IP)
			p.appFilter.srcIPMask = ipMask2int(ipf.src.IPNet.Mask)
			p.appFilter.dstPortRange = ipf.dst.ports
			p.appFilter.srcPortRange = ipf.src.ports

			return nil
		}
	}

	return nil
}

func (p *pdr) parseSDFFilter(ie *ie.IE) error {
	sdfFields, err := ie.SDFFilter()
	if err != nil {
		return err
	}

	flowDesc := sdfFields.FlowDescription
	if flowDesc == "" {
		return ErrOperationFailedWithReason("parse SDF Filter", "empty filter description")
	}

	log.WithFields(log.Fields{
		"Flow Description": flowDesc,
	}).Debug("Parsing Flow Description from SDF Filter")

	ipf, err := parseFlowDesc(flowDesc, int2ip(p.ueAddress).String())
	if err != nil {
		return errBadFilterDesc
	}

	if ipf.proto != reservedProto {
		p.appFilter.proto = ipf.proto
		p.appFilter.protoMask = math.MaxUint8
	}

	if p.srcIface == core {
		p.appFilter.dstIP = ip2int(ipf.dst.IPNet.IP)
		p.appFilter.dstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
		p.appFilter.srcIP = ip2int(ipf.src.IPNet.IP)
		p.appFilter.srcIPMask = ipMask2int(ipf.src.IPNet.Mask)
		p.appFilter.dstPortRange = ipf.dst.ports
		p.appFilter.srcPortRange = ipf.src.ports

		// FIXME: temporary workaround for SDF Filter,
		//  remove once we meet spec compliance
		if !p.appFilter.dstPortRange.isWildcardMatch() {
			p.appFilter.srcPortRange = p.appFilter.dstPortRange
			p.appFilter.dstPortRange = newWildcardPortRange()
		}
	} else if p.srcIface == access {
		p.appFilter.srcIP = ip2int(ipf.dst.IPNet.IP)
		p.appFilter.srcIPMask = ipMask2int(ipf.dst.IPNet.Mask)
		p.appFilter.dstIP = ip2int(ipf.src.IPNet.IP)
		p.appFilter.dstIPMask = ipMask2int(ipf.src.IPNet.Mask)
		// Ports are flipped for access PDRs
		p.appFilter.dstPortRange = ipf.src.ports
		p.appFilter.srcPortRange = ipf.dst.ports

		// FIXME: temporary workaround for SDF Filter,
		//  remove once we meet spec compliance
		if !p.appFilter.srcPortRange.isWildcardMatch() {
			p.appFilter.dstPortRange = p.appFilter.srcPortRange
			p.appFilter.srcPortRange = newWildcardPortRange()
		}
	}

	return nil
}

func (p *pdr) parsePDI(pdiIEs []*ie.IE, appPFDs map[string]appPFD, ippool *IPPool) error {
	for _, pdiIE := range pdiIEs {
		switch pdiIE.Type {
		case ie.UEIPAddress:
			if err := p.parseUEAddressIE(pdiIE, ippool); err != nil {
				log.Errorf("Failed to parse UE Address IE: %v", err)
				return err
			}
		case ie.SourceInterface:
			if err := p.parseSourceInterfaceIE(pdiIE); err != nil {
				log.Errorf("Failed to parse Source Interface IE: %v", err)
				return err
			}
		case ie.FTEID:
			if err := p.parseFTEID(pdiIE); err != nil {
				log.Errorf("Failed to parse F-TEID IE: %v", err)
				return err
			}
		}
	}

	// initialize application filter with UE address;
	// it can be overwritten by parseSDFFilter() later.
	if p.IsDownlink() && p.ueAddress != 0 {
		p.appFilter.dstIP = p.ueAddress
		p.appFilter.dstIPMask = math.MaxUint32 // /32
	} else if p.IsUplink() && p.ueAddress != 0 {
		p.appFilter.srcIP = p.ueAddress
		p.appFilter.srcIPMask = math.MaxUint32 // /32
	}

	// make another iteration because Application ID and SDF Filter depend on UE IP Address IE
	for _, ie2 := range pdiIEs {
		switch ie2.Type {
		case ie.ApplicationID:
			if err := p.parseApplicationID(ie2, appPFDs); err != nil {
				log.Errorf("Failed to parse Application ID IE: %v", err)
				return err
			}
		case ie.SDFFilter:
			if err := p.parseSDFFilter(ie2); err != nil {
				log.Errorf("Failed to parse SDF Filter IE: %v", err)
				return err
			}
		}
	}

	return nil
}

func (p *pdr) parsePDR(ie1 *ie.IE, seid uint64, appPFDs map[string]appPFD, ippool *IPPool) error {
	/* reset outerHeaderRemoval to begin with */
	outerHeaderRemoval := uint8(0)
	p.qerIDList = make([]uint32, 0)
	p.fseID = seid

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

	err = p.parsePDI(pdi, appPFDs, ippool)
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
	p.farID = farID // farID currently not being set <--- FIXIT/TODO/XXX
	/*p.qerID = qerID*/
	p.needDecap = outerHeaderRemoval

	return nil
}
