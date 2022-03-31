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
	Low  uint16
	High uint16
}

// newWildcardPortRange returns a portRange that matches on every possible port, i.e., implements
// no filtering.
func newWildcardPortRange() portRange {
	return portRange{
		Low:  0,
		High: math.MaxUint16,
	}
}

// newExactMatchPortRange returns a portRange that matches on exactly the given port.
func newExactMatchPortRange(port uint16) portRange {
	return portRange{
		Low:  port,
		High: port,
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
		Low:  low,
		High: high,
	}
}

func (pr portRange) String() string {
	return fmt.Sprintf("{%v-%v}", pr.Low, pr.High)
}

// Width returns the number of ports covered by this portRange.
func (pr portRange) Width() uint16 {
	// Need to handle the zero value.
	if pr.isWildcardMatch() {
		return math.MaxUint16
	} else {
		return pr.High - pr.Low + 1
	}
}

func (pr portRange) isWildcardMatch() bool {
	return pr.Low == 0 && pr.High == math.MaxUint16 ||
		pr.Low == 0 && pr.High == 0
}

func (pr portRange) isExactMatch() bool {
	return pr.Low == pr.High && pr.High != 0
}

func (pr portRange) isRangeMatch() bool {
	return !pr.isExactMatch() && !pr.isWildcardMatch()
}

// Returns portRange as an exact match, without checking if it is one. isExactMatch() must be true
// before calling asExactMatchUnchecked.
func (pr portRange) asExactMatchUnchecked() portRangeTernaryRule {
	return portRangeTernaryRule{port: pr.Low, mask: math.MaxUint16}
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

		for port := int(pr.Low); port <= int(pr.High); port++ {
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

		port := uint32(pr.Low) // Promote to higher bit width for greater-equals check.
		for port <= uint32(pr.High) {
			mask := portMask(uint16(port), pr.High)
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
	SrcIP        uint32
	DstIP        uint32
	SrcPortRange portRange
	DstPortRange portRange
	Proto        uint8

	SrcIPMask uint32
	DstIPMask uint32
	ProtoMask uint8
}

type pdr struct {
	SrcIface     uint8
	TunnelIP4Dst uint32
	TunnelTEID   uint32
	UeAddress    uint32

	SrcIfaceMask     uint8
	TunnelIP4DstMask uint32
	TunnelTEIDMask   uint32

	AppFilter applicationFilter

	Precedence  uint32
	PdrID       uint32
	FseID       uint64
	FseidIP     uint32
	CtrID       uint32
	FarID       uint32
	QerIDList   []uint32
	NeedDecap   uint8
	AllocIPFlag bool
}

func needAllocIP(ueIPaddr *ie.UEIPAddressFields) bool {
	if has2ndBit(ueIPaddr.Flags) && !has5thBit(ueIPaddr.Flags) {
		return false
	}

	return true
}

func (af applicationFilter) String() string {
	return fmt.Sprintf("ApplicationFilter(srcIP=%v/%x, dstIP=%v/%x, proto=%v/%x, srcPort=%v, dstPort=%v)",
		int2ip(af.SrcIP), af.SrcIPMask, int2ip(af.DstIP), af.DstIPMask, af.Proto,
		af.ProtoMask, af.SrcPortRange, af.DstPortRange)
}

func (p pdr) String() string {
	return fmt.Sprintf("PDR(id=%v, F-SEID=%v, SrcIface=%v, tunnelIPv4Dst=%v/%x, "+
		"tunnelTEID=%v/%x, UeAddress=%v, applicationFilter=%v, precedence=%v, F-SEID IP=%v, "+
		"counterID=%v, farID=%v, qerIDs=%v, needDecap=%v, AllocIPFlag=%v)",
		p.PdrID, p.FseID, p.SrcIface, int2ip(p.TunnelIP4Dst), p.TunnelIP4DstMask,
		p.TunnelTEID, p.TunnelTEIDMask, int2ip(p.UeAddress), p.AppFilter, p.Precedence,
		p.FseidIP, p.CtrID, p.FarID, p.QerIDList, p.NeedDecap, p.AllocIPFlag)
}

func (p pdr) IsAppFilterEmpty() bool {
	return p.AppFilter.Proto == 0 &&
		((p.IsUplink() && p.AppFilter.DstIP == 0 && p.AppFilter.DstPortRange.isWildcardMatch()) ||
			(p.IsDownlink() && p.AppFilter.SrcIP == 0 && p.AppFilter.SrcPortRange.isWildcardMatch()))
}

func (p pdr) IsUplink() bool {
	return p.SrcIface == access
}

func (p pdr) IsDownlink() bool {
	return p.SrcIface == core
}

func (p *pdr) parseUEAddressIE(ueAddrIE *ie.IE, ippool *IPPool) error {
	var ueIP4 net.IP

	ueIPaddr, err := ueAddrIE.UEIPAddress()
	if err != nil {
		return err
	}

	if needAllocIP(ueIPaddr) {
		/* alloc IPV6 if CHV6 is enabled : TBD */
		log.Infof("UPF should alloc UE IP for SEID %v. CHV4 flag set", p.FseID)

		ueIP4, err = ippool.LookupOrAllocIP(p.FseID)
		if err != nil {
			log.Errorln("failed to allocate UE IP")
			return err
		}

		log.Traceln("Found or allocated new IP", ueIP4, "from pool", ippool)

		p.AllocIPFlag = true
	} else {
		ueIP4 = ueIPaddr.IPv4Address
	}

	// Needed if SDF filter is bad or absent
	if len(ueIP4) != 4 {
		return ErrOperationFailedWithParam("parse UE Address IE",
			"IP address length", len(ueIP4))
	}

	p.UeAddress = ip2int(ueIP4)

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
		p.SrcIface = access
		p.SrcIfaceMask = 0xFF
	} else if srcIface == ie.SrcInterfaceCore {
		p.SrcIface = core
		p.SrcIfaceMask = 0xFF
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
		p.TunnelTEID = teid
		p.TunnelTEIDMask = 0xFFFFFFFF
		p.TunnelIP4Dst = ip2int(tunnelIPv4Address)
		p.TunnelIP4DstMask = 0xFFFFFFFF
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

		ipf, err := parseFlowDesc(flowDesc, int2ip(p.UeAddress).String())
		if err != nil {
			return errBadFilterDesc
		}

		if (p.SrcIface == access && ipf.direction == "out") ||
			(p.SrcIface == core && ipf.direction == "in") {
			logger.Debug("Found a matching flow description")

			if ipf.proto != reservedProto {
				p.AppFilter.Proto = ipf.proto
				p.AppFilter.ProtoMask = math.MaxUint8
			}
			// TODO: Verify assumption that flow description in case of PFD is to be taken as-is
			p.AppFilter.DstIP = ip2int(ipf.dst.IPNet.IP)
			p.AppFilter.DstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
			p.AppFilter.SrcIP = ip2int(ipf.src.IPNet.IP)
			p.AppFilter.SrcIPMask = ipMask2int(ipf.src.IPNet.Mask)
			p.AppFilter.DstPortRange = ipf.dst.ports
			p.AppFilter.SrcPortRange = ipf.src.ports

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

	ipf, err := parseFlowDesc(flowDesc, int2ip(p.UeAddress).String())
	if err != nil {
		return errBadFilterDesc
	}

	if ipf.proto != reservedProto {
		p.AppFilter.Proto = ipf.proto
		p.AppFilter.ProtoMask = math.MaxUint8
	}

	if p.SrcIface == core {
		p.AppFilter.DstIP = ip2int(ipf.dst.IPNet.IP)
		p.AppFilter.DstIPMask = ipMask2int(ipf.dst.IPNet.Mask)
		p.AppFilter.SrcIP = ip2int(ipf.src.IPNet.IP)
		p.AppFilter.SrcIPMask = ipMask2int(ipf.src.IPNet.Mask)
		p.AppFilter.DstPortRange = ipf.dst.ports
		p.AppFilter.SrcPortRange = ipf.src.ports

		// FIXME: temporary workaround for SDF Filter,
		//  remove once we meet spec compliance
		if !p.AppFilter.DstPortRange.isWildcardMatch() {
			p.AppFilter.SrcPortRange = p.AppFilter.DstPortRange
			p.AppFilter.DstPortRange = newWildcardPortRange()
		}
	} else if p.SrcIface == access {
		p.AppFilter.SrcIP = ip2int(ipf.dst.IPNet.IP)
		p.AppFilter.SrcIPMask = ipMask2int(ipf.dst.IPNet.Mask)
		p.AppFilter.DstIP = ip2int(ipf.src.IPNet.IP)
		p.AppFilter.DstIPMask = ipMask2int(ipf.src.IPNet.Mask)
		// Ports are flipped for access PDRs
		p.AppFilter.DstPortRange = ipf.src.ports
		p.AppFilter.SrcPortRange = ipf.dst.ports

		// FIXME: temporary workaround for SDF Filter,
		//  remove once we meet spec compliance
		if !p.AppFilter.SrcPortRange.isWildcardMatch() {
			p.AppFilter.DstPortRange = p.AppFilter.SrcPortRange
			p.AppFilter.SrcPortRange = newWildcardPortRange()
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
	if p.IsDownlink() && p.UeAddress != 0 {
		p.AppFilter.DstIP = p.UeAddress
		p.AppFilter.DstIPMask = math.MaxUint32 // /32
	} else if p.IsUplink() && p.UeAddress != 0 {
		p.AppFilter.SrcIP = p.UeAddress
		p.AppFilter.SrcIPMask = math.MaxUint32 // /32
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
	p.QerIDList = make([]uint32, 0)
	p.FseID = seid

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
				p.QerIDList = append(p.QerIDList, qerID)
			}
		}
	}
	/*qerID, err := ie1.QERID()
	if err != nil {
		log.Println("Could not read QER ID!")
	}*/

	p.Precedence = precedence
	p.PdrID = uint32(pdrID)
	p.FarID = farID // farID currently not being set <--- FIXIT/TODO/XXX
	/*p.qerID = qerID*/
	p.NeedDecap = outerHeaderRemoval

	return nil
}
