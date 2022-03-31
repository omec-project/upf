// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/omec-project/upf-epc/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vishvananda/netlink"
	"github.com/wmnsk/go-pfcp/ie"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math"
	"net"
	"os/exec"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	log "github.com/sirupsen/logrus"
)

type neighborCacheItem struct {
	nhopMAC          net.HardwareAddr
	routeCount       int
	updateModuleName string
	ogate            uint64
}

type aether struct {
	bess
	ownIp           *net.IPNet
	ueSubnet        *net.IPNet
	routeToFabric   netlink.Route
	gatewayIP       net.IP
	gatewayMAC      net.HardwareAddr
	datapathMAC     net.HardwareAddr
	addrUpdatesDone chan struct{}
	neighborCache   map[string]neighborCacheItem
}

func NewAether() *aether {
	return &aether{
		gatewayIP:       make(net.IP, net.IPv4len),
		gatewayMAC:      make(net.HardwareAddr, 6),
		datapathMAC:     make(net.HardwareAddr, 6),
		addrUpdatesDone: make(chan struct{}),
		neighborCache:   make(map[string]neighborCacheItem),
	}
}

const (
	// veth pair names. DO NOT MODIFY.
	datapathIfaceName   = "datapath"
	vethIfaceNameKernel = "fab"
	bessRoutingTable    = 201
	maxGates            = 8192

	// Time to wait for IP assignment on veth interface.
	vethIpDiscoveryTimeout = time.Second * 2

	// BESS module names.
	datapathIPLookModule = datapathIfaceName + "Routes"
)

const (
	moduleMethodAdd           = "add"
	moduleMethodDelete        = "delete"
	moduleMethodClear         = "clear"
	moduleMethodGetInitialArg = "get_initial_arg"
)

type interfaceClassification struct {
	// Match
	tunnelDstIp, tunnelDstIpMask uint32
	dstIp, dstIpMask             uint32
	ipProto, ipProtoMask         uint8
	dstPort, dstPortMask         uint16
	priority                     int64
	// Action
	srcIface uint8
	gate     uint64 // 0 pass, 1 fail
}

// SetUpfInfo is the entry point into the aether module.
func (a *aether) SetUpfInfo(u *upf, conf *Conf) {
	a.bess.SetUpfInfo(u, conf)

	var err error

	ctx, cancel := context.WithTimeout(context.Background(), vethIpDiscoveryTimeout)
	defer cancel()

	// IP packets to UE subnet are downlink, from core.
	_, a.ueSubnet, err = net.ParseCIDR(u.ippoolCidr)
	if err != nil {
		log.Fatalln(err)
	}

	// Setup MAC addresses, route configs, etc.
	if err = a.syncInterface(ctx, vethIfaceNameKernel); err != nil {
		log.Fatalf("could not sync addresses and routes of %v interface: %v", vethIfaceNameKernel, err)
	}

	if err = a.startInterfaceWatchTask(vethIfaceNameKernel); err != nil {
		log.Fatalf("could not start watch task on %v interface: %v", vethIfaceNameKernel, err)
	}

	// Needed for legacy code. Remove once refactored.
	u.coreIP = net.IPv4zero.To4()
	u.accessIP = a.ownIp.IP

	if u.coreIP == nil || u.accessIP == nil {
		log.Fatalln("upf IP is not a IPv4 address")
	}

	u.enableFlowMeasure = true

	if err = a.setupInterfaceClassification(); err != nil {
		log.Fatalln(err)
	}

	if err = a.setupBpfRules(); err != nil {
		log.Fatalln(err)
	}
}

func (a *aether) Exit() {
	a.bess.Exit()
	close(a.addrUpdatesDone)
}

func (a *aether) SendMsgToUPF(method upfMsgType, all PacketForwardingRules, updated PacketForwardingRules) uint8 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	fars := all.fars
	if method == upfMsgTypeMod {
		fars = updated.fars
	}

	for _, far := range fars {
		log.Traceln(method, far)
		if far.Forwards() && far.dstIntf != ie.DstInterfaceAccess {
			// Not a downlink rule.
			log.Traceln("skipping", far)
			// TODO: insert uplink route?
			continue
		}
		enbIP := &net.IPNet{
			IP:   utils.Uint32ToIp4(far.tunnelIP4Dst),
			Mask: net.CIDRMask(net.IPv4len*8, net.IPv4len*8), // a /32 mask
		}
		var err error
		var nhmac net.HardwareAddr
		// Check if received eNB IP is in fabric subnet, i.e. directly attached.
		if a.routeToFabric.Dst.Contains(enbIP.IP) {
			// Resolve eNB MAC and install "bridging" entry.
			if err = ping(ctx, enbIP.IP); err != nil {
				log.Errorln("ping failed", err)
				return ie.CauseRequestRejected
			}
			nhmac, err = a.resolveNeighbor(ctx, 0, enbIP.IP)
			if err != nil {
				log.Errorln("resolveNeighbor failed", err)
				return ie.CauseRequestRejected
			}
		} else {
			// Install routing entry with fabric GW next hop.
			nhmac = a.gatewayMAC
		}

		switch method {
		case upfMsgTypeAdd:
			if err = a.addIPLookupRule(ctx, enbIP, nhmac); err != nil {
				log.Errorln("addIPLookupRule failed", err)
				return ie.CauseRequestRejected
			}
			log.Tracef("Added eNB IP route to %v via nhop %v", enbIP, nhmac)
		case upfMsgTypeMod:
		case upfMsgTypeDel:
			if err = a.deleteIPLookupRule(ctx, enbIP); err != nil {
				log.Errorln("deleteIPLookupRule failed", err)
				return ie.CauseRequestRejected
			}
			log.Tracef("Removed eNB IP route to %v via nhop %v", enbIP, nhmac)
		default:
			log.Errorln("unknown method", method)
		}
	}

	return a.bess.SendMsgToUPF(method, all, updated)
}

// Poll the given interface until an IPv4 address is found on it.
func waitForIpConfigured(ctx context.Context, link netlink.Link) (*net.IPNet, error) {
	ticker := time.NewTicker(time.Millisecond * 250)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			addrs, err := netlink.AddrList(link, unix.AF_INET)
			if err != nil {
				return nil, err
			}
			// TODO(max): filter addresses?
			for _, addr := range addrs {
				return addr.IPNet, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (a *aether) syncInterface(ctx context.Context, iface string) (err error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return
	}

	// Own MAC.
	copy(a.datapathMAC, link.Attrs().HardwareAddr)

	if len(a.datapathMAC) != 6 {
		return ErrInvalidArgumentWithReason("a.datapathMAC", a.datapathMAC, "invalid mac address")
	}

	// Fetch own IPv4 address.
	a.ownIp, err = waitForIpConfigured(ctx, link)
	a.ownIp.IP = a.ownIp.IP.To4() // Make sure it's in 4 byte format.
	if a.ownIp == nil || a.ownIp.IP == nil {
		return ErrOperationFailedWithReason("a.ownIp", "found no IPv4 address")
	}
	log.Tracef("found IP %v on local %v interface", a.ownIp, iface)

	routes, err := netlink.RouteList(link, unix.AF_INET)
	if err != nil {
		return
	}

	for _, r := range routes {
		log.Tracef("route: %+v", r)

		// Bridging
		if r.Scope == netlink.SCOPE_LINK {
			log.Traceln("Found route with scope link:", r)
			a.routeToFabric = r
			break
		}
	}

	if a.routeToFabric.Dst == nil {
		return ErrOperationFailedWithReason("syncInterface", "found no route to fabric")
	}

	// Fetch default route and gateway from alternative routing table.
	altRoutes, err := netlink.RouteListFiltered(
		unix.AF_INET,
		&netlink.Route{Table: bessRoutingTable},
		netlink.RT_FILTER_TABLE)
	if err != nil {
		return
	}
	for _, r := range altRoutes {
		if r.Gw != nil {
			log.Traceln("Found default route with gateway IP:", r)
			copy(a.gatewayIP, r.Gw.To4())
			break
		}
	}

	if a.gatewayIP == nil {
		return ErrOperationFailedWithReason("syncInterface", "found no gateway to fabric")
	}

	a.gatewayMAC, err = a.resolveNeighbor(ctx, link.Attrs().Index, a.gatewayIP)
	if err != nil {
		return
	}

	log.Traceln("resolved neighbor mac", a.gatewayMAC)

	return a.setupRoutingRules()
}

func (a *aether) startInterfaceWatchTask(iface string) (err error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return
	}

	updates := make(chan netlink.AddrUpdate)

	if err = netlink.AddrSubscribe(updates, a.addrUpdatesDone); err != nil {
		return
	}

	go a.interfaceWatchTask(link, updates)

	return
}

func (a *aether) interfaceWatchTask(link netlink.Link, updates <-chan netlink.AddrUpdate) {
	for {
		update, ok := <-updates
		if !ok {
			log.Infof("Address update subscriber channel for %v interface closed", link.Attrs().Name)
			return
		}
		if update.LinkIndex != link.Attrs().Index {
			// Not for this interface, ignore.
			continue
		}

		// We don't handle runtime address change, and instead abort.
		log.Fatalf("IP address for %v interface changed: %+v. Reboot required", link.Attrs().Name, update)
	}
}

func ping(ctx context.Context, dst net.IP) error {
	c := exec.CommandContext(ctx, "ping", "-c", "1", dst.String())
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		log.Errorln(err, stdout.String(), stderr.String())
		return err
	}

	return nil
}

func (a *aether) resolveNeighbor(ctx context.Context, linkIndex int, ip net.IP) (net.HardwareAddr, error) {
	// Trigger ARP lookup for gateway IP.
	if err := ping(ctx, ip); err != nil {
		log.Errorf("Ping to %v failed: %v", ip, err)
		return nil, err
	}

	neighs, err := netlink.NeighList(linkIndex, unix.AF_INET)
	if err != nil {
		return nil, err
	}

	for _, n := range neighs {
		log.Tracef("%+v", n)
		if n.State == netlink.NUD_FAILED || n.State == netlink.NUD_NOARP {
			continue
		}
		if n.IP.Equal(ip) {
			return n.HardwareAddr, nil
		}
	}

	return nil, ErrNotFound("neighbor " + ip.String())
}

func (a *aether) setupRoutingRules() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// TODO(max): this should be a bridging-like entry
	if err = a.addIPLookupRule(ctx, a.routeToFabric.Dst, a.gatewayMAC); err != nil {
		return
	}

	// Default route over Fabric gateway for encaped uplink traffic.
	defaultRoute := &net.IPNet{
		IP:   net.IPv4zero,
		Mask: net.CIDRMask(0, net.IPv4len*8),
	}
	if err = a.addIPLookupRule(ctx, defaultRoute, a.gatewayMAC); err != nil {
		return
	}

	return
}

func (a *aether) linkBessModules(ctx context.Context, m1 string, ogate uint64, m2 string, igate uint64) (err error) {
	req := &pb.ConnectModulesRequest{
		M1:               m1,
		M2:               m2,
		Ogate:            ogate,
		Igate:            igate,
		SkipDefaultHooks: false,
	}

	if _, err = a.client.ConnectModules(ctx, req); err != nil {
		log.Errorf("Could not link modules with request %+v: %v", req, err)
		return
	}

	return
}

func (a *aether) findNextFreeOgate(ctx context.Context, module string) (ogate uint64, err error) {
	req := &pb.GetModuleInfoRequest{Name: module}
	resp, err := a.client.GetModuleInfo(ctx, req)
	if err != nil {
		return
	}

outer:
	for i := uint64(0); i < maxGates; i++ {
		for _, og := range resp.Ogates {
			if og.Ogate == i {
				continue outer
			}
		}
		// No collision found, this ogate is free.
		return i, nil
	}

	return 0, ErrNotFound("free ogate")
}

func (a *aether) addIPLookupRule(ctx context.Context, dst *net.IPNet, nhopMAC net.HardwareAddr) (err error) {
	ones, zeros := dst.Mask.Size()
	if ones+zeros == 0 {
		return ErrInvalidArgumentWithReason("addIPLookupRule", dst, "not a CIDR mask")
	}

	// Create next hop dst MAC packet data update module and add to cache, if needed.
	n, exists := a.neighborCache[dst.String()]
	log.Tracef("neighbor cache for %v exists: %v, %+v", dst, exists, n)
	if !exists {
		n.nhopMAC = nhopMAC
		n.updateModuleName, err = a.createDstMacUpdateModule(ctx, dst, nhopMAC)
		if err != nil {
			return err
		}

		n.ogate, err = a.findNextFreeOgate(ctx, datapathIPLookModule)
		if err != nil {
			return err
		}

		// Place module in between the IP lookup and merge modules.
		if err = a.linkBessModules(ctx, datapathIPLookModule, n.ogate, n.updateModuleName, 0); err != nil {
			return err
		}
		if err = a.linkBessModules(ctx, n.updateModuleName, 0, datapathIfaceName+"Merge", 0); err != nil {
			return err
		}

		// Insert lookup rule.
		msg := &pb.IPLookupCommandAddArg{
			Prefix:    dst.IP.Mask(dst.Mask).String(), // Need to clear lower bits.
			PrefixLen: uint64(ones),
			Gate:      n.ogate,
		}
		if err = a.processIPLookup(ctx, msg, moduleMethodAdd); err != nil {
			return err
		}
	}
	n.routeCount++
	a.neighborCache[dst.String()] = n

	return nil
}

func (a *aether) deleteIPLookupRule(ctx context.Context, dst *net.IPNet) error {
	ones, zeros := dst.Mask.Size()
	if ones+zeros == 0 {
		return ErrInvalidArgumentWithReason("deleteIPLookupRule", dst, "not a CIDR mask")
	}

	// Check if neighbor has a module and delete if necessary.
	n, exists := a.neighborCache[dst.String()]
	log.Tracef("neighbor cache for %v exists: %v, %+v", dst, exists, n)
	if exists {
		n.routeCount--
		a.neighborCache[dst.String()] = n

		if n.routeCount == 0 {
			delete(a.neighborCache, dst.String())
			if err := a.deleteModule(ctx, n.updateModuleName); err != nil {
				return err
			}
			// Delete lookup rule.
			msg := &pb.IPLookupCommandDeleteArg{
				Prefix:    dst.IP.String(), // Need to clear lower bits.
				PrefixLen: uint64(ones),
			}
			if err := a.processIPLookup(ctx, msg, moduleMethodDelete); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *aether) processIPLookup(ctx context.Context, msg proto.Message, method string) error {
	switch method {
	case moduleMethodAdd:
		fallthrough
	case moduleMethodDelete:
		fallthrough
	case moduleMethodClear:
	default:
		return ErrInvalidArgumentWithReason("method", method, "invalid method name")
	}

	any, err := anypb.New(msg)
	if err != nil {
		log.Error("Error marshalling the rule", msg, err)
		return err
	}

	// IPLookup module is not thread-safe, need to pause processing.
	if err := a.pauseBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

	resp, err := a.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: datapathIPLookModule,
		Cmd:  method,
		Arg:  any,
	})

	if err != nil {
		log.Errorf("processIPLookup ModuleCommand RPC failed with err: %v\n", err)
		return err
	}

	if resp.GetError() != nil && resp.GetError().Code != 0 {
		log.Errorf("processIPLookup %v request '%+v' failed with err: %v\n", method, msg, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	if err := a.resumeBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

	log.Tracef("proccesed IPLookup %v request %+v", method, msg)

	return nil
}

func (a *aether) createDstMacUpdateModule(ctx context.Context, ip *net.IPNet, nhop net.HardwareAddr) (string, error) {
	tmp := make([]byte, 8) // 64 bit
	copy(tmp[2:], nhop)

	arg := &pb.UpdateArg{Fields: []*pb.UpdateArg_Field{
		{Offset: 0, Size: 6, Value: binary.BigEndian.Uint64(tmp)},
	}}

	moduleName := datapathIfaceName + "DstIP" + ip.String() + "DstEther" + nhop.String()

	return moduleName, a.createModule(ctx, moduleName, "Update", arg)
}

func (a *aether) deleteDstMacUpdateModule(ctx context.Context, ip *net.IPNet, nhop net.HardwareAddr) (err error) {
	moduleName := datapathIfaceName + "DstIP" + ip.String() + "DstEther" + nhop.String()

	return a.deleteModule(ctx, moduleName)
}

func (a *aether) createModule(ctx context.Context, moduleName, moduleClass string, args proto.Message) error {
	any, err := anypb.New(args)
	if err != nil {
		log.Error("Error marshalling the rule", args, err)
		return err
	}

	req := &pb.CreateModuleRequest{
		Name:   moduleName,
		Mclass: moduleClass,
		Arg:    any,
	}

	resp, err := a.client.CreateModule(ctx, req)

	if err != nil {
		log.Errorf("CreateModule RPC failed with err: %v\n", err)
		return err
	}

	if resp.GetError() != nil && resp.GetError().Code != 0 {
		log.Errorf("CreateModule request '%+v' failed with err: %v\n", req, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	log.Tracef("Created new module %v of type %v", moduleName, moduleClass)

	return nil
}

func (a *aether) deleteModule(ctx context.Context, moduleName string) error {
	req := &pb.DestroyModuleRequest{
		Name: moduleName,
	}

	resp, err := a.client.DestroyModule(ctx, req)

	if err != nil {
		log.Errorf("DestroyModule RPC failed with err: %v\n", err)
		return err
	}

	if resp.GetError() != nil && resp.GetError().Code != 0 {
		log.Errorf("DestroyModule request '%+v' failed with err: %v\n", req, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	log.Tracef("Destroyed module %v", moduleName)

	return nil
}

func (a *aether) sessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) (err error) {
	// TODO: implement
	log.Traceln("sessionStats are not implemented in aether pipeline")
	return
}

func (a *aether) setupBpfRules() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// Do not modify. Hard-coded gates from aether.bess pipeline.
	const (
		ueTrafficPassGate     = 0
		signalTrafficPassGate = 8192 - 1 // MAX_GATE - 1
	)

	// Pass-through filter for GTPU UE traffic.
	ueFilter := "ip and dst host " + a.ownIp.IP.String() + " and udp dst port 2152"
	if err = a.addBpfRule(ctx, ueFilter, -ueTrafficPassGate, ueTrafficPassGate); err != nil {
		return
	}

	return
}

func (a *aether) pauseBessWorkers(ctx context.Context) error {
	resp, err := a.client.PauseAll(ctx, &pb.EmptyRequest{})
	if err != nil || resp.GetError() != nil {
		log.Errorf("PauseAll rpc failed with resp: %v, err: %v\n", resp, err)
		return err
	}

	return nil
}

func (a *aether) resumeBessWorkers(ctx context.Context) error {
	resp, err := a.client.ResumeAll(ctx, &pb.EmptyRequest{})
	if err != nil || resp.GetError() != nil {
		log.Errorf("ResumeAll rpc failed with resp: %v, err: %v\n", resp, err)
		return err
	}

	return nil
}

func (a *aether) addBpfRule(ctx context.Context, filter string, priority, gate int64) error {
	f := pb.BPFArg_Filter{
		Priority: priority,
		Filter:   filter,
		Gate:     gate,
	}
	bpfArg := &pb.BPFArg{Filters: []*pb.BPFArg_Filter{&f}}

	err := a.processBpf(ctx, bpfArg, moduleMethodAdd)
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

func (a *aether) processBpf(ctx context.Context, msg proto.Message, method string) error {
	switch method {
	case moduleMethodAdd:
		fallthrough
	case moduleMethodDelete:
		fallthrough
	case moduleMethodClear:
		fallthrough
	case moduleMethodGetInitialArg:
	default:
		return ErrInvalidArgumentWithReason("method", method, "invalid method name")
	}

	any, err := anypb.New(msg)
	if err != nil {
		log.Error("Error marshalling the rule", msg, err)
		return err
	}

	// BPF module is not thread-safe, need to pause processing.
	if err = a.pauseBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

	resp, err := a.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: datapathIfaceName + "FastBPF",
		Cmd:  method,
		Arg:  any,
	})

	if err != nil {
		log.Errorf("processBpf ModuleCommand RPC failed with err: %v\n", err)
		return err
	}

	if resp.GetError() != nil && resp.GetError().Code != 0 {
		log.Errorf("processBpf %v request '%+v' failed with err: %v\n", method, msg, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	if err = a.resumeBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

// setupInterfaceClassification inserts the necessary interface classification rules.
func (a *aether) setupInterfaceClassification() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// GTP encaped packets directly to UPF (outer IP dst) are uplink, from access.
	ifc := interfaceClassification{
		priority: 40,
		// Presence of a tunnel IP implies an outer UDP header and port 2152, as verified by the GTP
		// parser. No need (and possibility) to match on them here.
		tunnelDstIp:     ip2int(a.ownIp.IP),
		tunnelDstIpMask: math.MaxUint32,

		gate:     0,
		srcIface: access,
	}
	if err = a.addInterfaceClassification(ctx, ifc); err != nil {
		log.Errorln(err)
		return
	}

	// Packets to UEs are downlink, from core.
	ifc = interfaceClassification{
		priority:  30,
		dstIp:     ip2int(a.ueSubnet.IP),
		dstIpMask: ipMask2int(a.ueSubnet.Mask),

		gate:     0,
		srcIface: core,
	}
	if err = a.addInterfaceClassification(ctx, ifc); err != nil {
		log.Errorln(err)
		return
	}

	// Other packets addressed to the UPF are packet ins.
	ifc = interfaceClassification{
		priority:  1,
		dstIp:     ip2int(a.ownIp.IP),
		dstIpMask: math.MaxUint32,

		gate:     0,
		srcIface: 0,
	}
	if err = a.addInterfaceClassification(ctx, ifc); err != nil {
		log.Errorln(err)
		return
	}

	return
}

func (a *aether) addInterfaceClassification(ctx context.Context, ifc interfaceClassification) error {
	f := &pb.WildcardMatchCommandAddArg{
		Gate:     ifc.gate,
		Priority: ifc.priority,
		Values: []*pb.FieldData{
			intEnc(uint64(ifc.tunnelDstIp)), /* tunnel_ipv4_dst */
			intEnc(uint64(ifc.dstIp)),       /* dst_ip */
			intEnc(uint64(ifc.ipProto)),     /* ip_proto */
			intEnc(uint64(ifc.dstPort)),     /* dst_port */
		},
		Masks: []*pb.FieldData{
			intEnc(uint64(ifc.tunnelDstIpMask)), /* tunnel_ipv4_dst mask */
			intEnc(uint64(ifc.dstIpMask)),       /* dst_ip mask */
			intEnc(uint64(ifc.ipProtoMask)),     /* ip_proto mask */
			intEnc(uint64(ifc.dstPortMask)),     /* dst_port mask */
		},
		Valuesv: []*pb.FieldData{
			intEnc(uint64(ifc.srcIface)), /* src_iface */
		},
	}

	err := a.processInterfaceClassification(ctx, f, moduleMethodAdd)
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

func (a *aether) processInterfaceClassification(ctx context.Context, msg proto.Message, method string) error {
	if method != moduleMethodAdd && method != moduleMethodDelete && method != moduleMethodClear {
		return ErrInvalidArgumentWithReason("method", method, "invalid method name")
	}

	any, err := anypb.New(msg)
	if err != nil {
		log.Println("Error marshalling the rule", msg, err)
		return err
	}

	resp, err := a.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "interfaceClassification",
		Cmd:  method,
		Arg:  any,
	})

	if err != nil {
		log.Errorf("interfaceClassification %v RPC failed with err: %v\n", method, err)
		return err
	}

	if resp.GetError() != nil && resp.GetError().Code != 0 {
		log.Errorf("interfaceClassification %v request '%+v' failed with err: %v\n", method, msg, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	log.Tracef("%ved interfaceClassification '%+v'", method, msg)

	return nil
}
