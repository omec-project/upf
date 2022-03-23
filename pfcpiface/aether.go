// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math"
	"net"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	log "github.com/sirupsen/logrus"
)

type aether struct {
	bess
	ownIp        net.IP
	ueSubnet     *net.IPNet
	fabricSubnet *net.IPNet
	datapathMAC  []byte
}

const (
	// IP protocol types.
	TcpProto = 6
	UdpProto = 17

	// veth pair names. DO NOT MODIFY.
	datapathIfaceName   = "datapath"
	vethIfaceNameKernel = "fab"

	// Time to wait for IP assignment on veth interface.
	vethIpDiscoveryTimeout = time.Second * 2
)

const (
	moduleMethodAdd           = "add"
	moduleMethodDelete        = "delete"
	moduleMethodClear         = "clear"
	moduleMethodGetInitialArg = "get_initial_arg"
)

type interfaceClassification struct {
	// Match
	dstIp, dstIpMask     uint32
	ipProto, ipProtoMask uint8
	dstPort, dstPortMask uint16
	priority             int64
	// Action
	srcIface uint8
	gate     uint64 // 0 pass, 1 fail
}

func (a *aether) SetUpfInfo(u *upf, conf *Conf) {
	a.bess.SetUpfInfo(u, conf)

	var err error

	// TODO(max): make sure we're not getting a IPv6 address.
	// Wait for external assignment of veth IP address and store it.
	if a.ownIp, err = waitForIpConfigured(vethIfaceNameKernel, vethIpDiscoveryTimeout); err != nil {
		log.Fatalf("could not get IP on %v interface: %v", vethIfaceNameKernel, err)
	}

	// Truncate slice to 4 bytes for later use.
	a.ownIp = a.ownIp.To4()
	if a.ownIp == nil {
		log.Fatalln("upf IP is not a IPv4 address")
	}

	// IP packets to UE subnet are downlink, from core.
	_, a.ueSubnet, err = net.ParseCIDR(u.ippoolCidr)
	if err != nil {
		log.Fatalln(err)
	}

	// TODO: read from DPDK somehow
	a.datapathMAC = []byte{0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa}
	//a.datapathMAC = []byte{0x0c, 0xc4, 0x7a, 0x19, 0x6d, 0xca}
	if len(a.datapathMAC) != 6 {
		log.Fatalln("invalid mac address", a.datapathMAC)
	}

	// Setup route configs.
	if err = a.syncRoutes(vethIfaceNameKernel); err != nil {
		log.Fatalf("could not get routes on %v interface: %v", vethIfaceNameKernel, err)
	}

	// Needed for legacy code. Remove once refactored.
	u.coreIP = net.IPv4zero.To4()
	u.accessIP = a.ownIp

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

func waitForIpConfigured(iface string, timeout time.Duration) (net.IP, error) {
	deadline := time.After(timeout)

	ticker := time.NewTicker(time.Millisecond * 250)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ip, err := GetUnicastAddressFromInterface(iface)
			if err == nil {
				return ip, nil
			}
		case <-deadline:
			return nil, errTimeout
		}
	}
}

func (a *aether) syncRoutes(iface string) (err error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return
	}

	routes, err := netlink.RouteList(link, unix.AF_INET)
	if err != nil {
		return
	}

	for _, r := range routes {
		if r.Scope == netlink.SCOPE_LINK {
			a.fabricSubnet = r.Dst

			log.Traceln("Found route with scope link:", r)
		}
	}

	if a.fabricSubnet == nil {
		return ErrOperationFailedWithReason("syncRoutes", "found no fabric route")
	}

	return a.setupRoutingRules()
}

func (a *aether) setupRoutingRules() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	if err = a.addIPLookupRule(ctx, *a.fabricSubnet, 0); err != nil {
		return
	}

	// TODO(max): create next hop dmac modules and link
	//return ErrUnsupported("setupRoutingRules", "not implemented")

	return
}

func (a *aether) addIPLookupRule(ctx context.Context, dst net.IPNet, gate uint64) error {
	ones, zeros := dst.Mask.Size()
	if ones+zeros == 0 {
		return ErrInvalidArgumentWithReason("addIPLookupRule", dst, "not a CIDR mask")
	}

	msg := &pb.IPLookupCommandAddArg{
		Prefix:    dst.IP.String(),
		PrefixLen: uint64(ones),
		Gate:      gate,
	}

	return a.processIPLookup(ctx, msg, moduleMethodAdd)
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
		Name: datapathIfaceName + "Routes",
		Cmd:  method,
		Arg:  any,
	})

	if err != nil {
		log.Errorf("processIPLookup ModuleCommand RPC failed with err: %v\n", err)
		return err
	}

	if resp.GetError() != nil {
		log.Errorf("processIPLookup method failed with resp: %v, err: %v\n", resp, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	if err := a.resumeBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

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
	ueFilter := "ip and dst host " + a.ownIp.String() + " and udp dst port 2152"
	if err = a.addBpfRule(ctx, ueFilter, -ueTrafficPassGate, ueTrafficPassGate); err != nil {
		return
	}

	// ARP, ICMP and DHCP filter to veth interface.
	const signalFilter = "arp or icmp or (udp and (port 67 or port 68))"
	if err = a.addBpfRule(ctx, signalFilter, -signalTrafficPassGate, signalTrafficPassGate); err != nil {
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
	if err := a.pauseBessWorkers(ctx); err != nil {
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

	if resp.GetError() != nil {
		log.Errorf("processBpf method failed with resp: %v, err: %v\n", resp, resp.GetError())
		return status.Error(codes.Code(resp.GetError().Code), resp.GetError().Errmsg)
	}

	if err := a.resumeBessWorkers(ctx); err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

// setupInterfaceClassification inserts the necessary interface classification rules.
func (a *aether) setupInterfaceClassification() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// Other GTP packets directly to UPF are uplink, from access.
	ifc := interfaceClassification{
		priority:    40,
		dstIp:       ip2int(a.ownIp),
		dstIpMask:   math.MaxUint32,
		ipProto:     UdpProto,
		ipProtoMask: math.MaxUint8,
		dstPort:     tunnelGTPUPort,
		dstPortMask: math.MaxUint16,

		gate:     0,
		srcIface: access,
	}
	if err = a.addInterfaceClassification(ctx, ifc); err != nil {
		log.Errorln(err)
		return
	}

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
		dstIp:     ip2int(a.ownIp),
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
			intEnc(uint64(ifc.dstIp)),   /* dst_ip */
			intEnc(uint64(ifc.ipProto)), /* ip_proto */
			intEnc(uint64(ifc.dstPort)), /* dst_port */
		},
		Masks: []*pb.FieldData{
			intEnc(uint64(ifc.dstIpMask)),   /* dst_ip mask */
			intEnc(uint64(ifc.ipProtoMask)), /* ip_proto mask */
			intEnc(uint64(ifc.dstPortMask)), /* dst_port mask */
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

	if err != nil || resp.GetError() != nil {
		log.Errorf("interfaceClassification method failed with resp: %v, err: %v\n", resp, err)
	}

	return nil
}
