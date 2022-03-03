// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mdlayher/packet"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	_ "github.com/google/gopacket/layers"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math"
	"net"
)

const (
	// IP protocol types
	TcpProto = layers.IPProtocolTCP
	UdpProto = layers.IPProtocolUDP
)

var (
	// dummyAddr is used when writing packets to the AF_PACKET socket. In SOCK_RAW mode the
	// sockaddr_ll struct is not required as the frame already contains an Ethernet header, but the
	// packet library still requires passing in an address.
	dummyAddr = packet.Addr{HardwareAddr: []byte{0, 0, 0, 0, 0, 0}}
	useUnix   = true
)

type aether struct {
	bess
	accessIP    net.IP
	ueSubnet    *net.IPNet
	pktioSock   *packet.Conn
	unixSock    *net.UnixConn
	datapathMAC []byte
}

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

func (a *aether) setUpfInfo(u *upf, conf *Conf) {
	a.bess.setUpfInfo(u, conf)
	var err error

	// GTP-U packets from eNB are uplink, to core.
	a.accessIP, _, err = net.ParseCIDR(conf.P4rtcIface.AccessIP)
	if err != nil {
		log.Fatalf("unable to parse IP %v that we should parse", conf.P4rtcIface.AccessIP)
	}
	// Truncate slice to 4 bytes for later use.
	a.accessIP = a.accessIP.To4()
	if a.accessIP == nil {
		log.Fatalln("access IP is not a IPv4 address")
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

	u.coreIP = net.ParseIP(net.IPv4zero.String())
	u.accessIP = a.accessIP

	u.enableFlowMeasure = true

	if err = a.setupInterfaceClassification(); err != nil {
		log.Fatalln(err)
	}

	if err = a.setupPktioSocket(conf); err != nil {
		log.Fatalln(err)
	}
}

func (a *aether) setupPktioSocket(conf *Conf) error {
	if useUnix {
		// unix
		addr, err := net.ResolveUnixAddr("unixpacket", "/tmp/sockets/pktio.sock")
		if err != nil {
			return err
		}
		a.unixSock, err = net.DialUnix("unixpacket", nil, addr)
		if err != nil {
			return err
		}
	} else {
		// veth
		inf, err := net.InterfaceByName(conf.DataplaneInterface)
		if err != nil {
			return err
		}

		a.pktioSock, err = packet.Listen(inf, packet.Raw, unix.ETH_P_ALL, nil)
		if err != nil {
			return err
		}

		err = a.pktioSock.SetPromiscuous(true)
		if err != nil {
			return err
		}
	}

	go func() {
		err := a.pktioRecvLoop()
		if err != nil {
			log.Errorln("pktioRecvLoop:", err)
		}
	}()

	return nil
}

// uds implements the gopacket.PacketDataSource interface.
type uds struct {
	conn *net.UnixConn
}

func (u *uds) ReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	buf := make([]byte, 2000)
	n, raddr, err := u.conn.ReadFrom(buf)
	if err != nil {
		return
	}

	ci.CaptureLength = n
	ci.Length = n
	ci.AncillaryData = append(ci.AncillaryData, raddr)

	data = append(data, buf[:n]...)

	return
}

// pds implements the gopacket.PacketDataSource interface.
type pds struct {
	conn *packet.Conn
}

func (p *pds) ReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	buf := make([]byte, 2000)
	n, raddr, err := p.conn.ReadFrom(buf)
	if err != nil {
		return
	}

	ci.CaptureLength = n
	ci.Length = n
	ci.AncillaryData = append(ci.AncillaryData, raddr)

	data = append(data, buf[:n]...)

	return
}

func (a *aether) pktioRecvLoop() error {
	var src *gopacket.PacketSource
	if useUnix {
		// unix
		defer a.unixSock.Close()
		// Start decoding from Ethernet later.
		src = gopacket.NewPacketSource(&uds{conn: a.unixSock}, layers.LayerTypeEthernet)
	} else {
		// veth
		defer a.pktioSock.Close()
		s := pds{
			conn: a.pktioSock,
		}
		// Start decoding from Ethernet later.
		src = gopacket.NewPacketSource(&s, layers.LayerTypeEthernet)
	}

	for pkt := range src.Packets() {
		log.Warnln(pkt)
		ethLayer := pkt.Layer(layers.LayerTypeEthernet)
		if ethLayer == nil {
			log.Warnln("Unknown packet", pkt)
			continue
		}
		eth, ok := ethLayer.(*layers.Ethernet)
		if !ok {
			log.Warnln("failed to parse Ethernet layer", ethLayer)
			continue
		}
		// TODO: check dst mac for us or broadcast

		if arpLayer := pkt.Layer(layers.LayerTypeARP); arpLayer != nil {
			if err := a.handleARP(pkt, arpLayer); err != nil {
				log.Errorln(err)
			}
			continue
		}
		if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
			if err := a.handleICMP(pkt, eth, icmpLayer); err != nil {
				log.Errorln(err)
			}
			continue
		}
	}

	return nil
}

func (a *aether) sendPacketOut(buf gopacket.SerializeBuffer) error {
	if useUnix {
		// unix
		_, err := a.unixSock.Write(buf.Bytes())
		if err != nil {
			return err
		}
	} else {
		// veth
		_, err := a.pktioSock.WriteTo(buf.Bytes(), &dummyAddr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *aether) handleARP(pkt gopacket.Packet, arpLayer gopacket.Layer) error {
	arp, ok := arpLayer.(*layers.ARP)
	if !ok {
		return ErrInvalidArgument("handleARP", arpLayer)
	}

	if arp.ProtAddressSize != net.IPv4len {
		log.Warnln("Unexpected ARP proto addr size", arp.ProtAddressSize)
		return nil
	}
	if int(arp.HwAddressSize) != len(layers.EthernetBroadcast) {
		log.Warnln("Unexpected ARP hw addr size", arp.HwAddressSize)
		return nil
	}

	if arp.Operation == layers.ARPRequest {
		// Not for us.
		if !a.accessIP.Equal(arp.DstProtAddress) {
			log.Warnln("Unexpected ARP proto dst address", arp.DstProtAddress)
			return nil
		}

		ethResp := &layers.Ethernet{
			BaseLayer:    layers.BaseLayer{},
			SrcMAC:       a.datapathMAC,
			DstMAC:       arp.SourceHwAddress,
			EthernetType: layers.EthernetTypeARP,
		}
		arpResp := &layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			HwAddressSize:     arp.HwAddressSize,
			ProtAddressSize:   arp.ProtAddressSize,
			Operation:         layers.ARPReply,
			SourceHwAddress:   a.datapathMAC,
			SourceProtAddress: a.accessIP,
			DstHwAddress:      arp.SourceHwAddress,
			DstProtAddress:    arp.SourceProtAddress,
		}

		buf := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}, ethResp, arpResp)
		if err != nil {
			log.Error(err)
			log.Errorln(gopacket.LayerDump(ethResp))
			log.Errorln(gopacket.LayerDump(arpResp))
			return err
		}

		err = a.sendPacketOut(buf)
		if err != nil {
			return err
		}
		txPkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
		log.Tracef("Sent ARP reply: %v", txPkt)
	} else if arp.Operation == layers.ARPReply {

	} else {
		log.Warnln("Unknown ARP operation", arp.Operation)
		return nil
	}

	return nil
}

func (a *aether) handleICMP(pkt gopacket.Packet, eth *layers.Ethernet, icmpLayer gopacket.Layer) error {
	ipv4Layer := pkt.Layer(layers.LayerTypeIPv4)
	if ipv4Layer == nil {
		return ErrInvalidArgument("handleICMP", pkt)
	}
	ipv4, ok := ipv4Layer.(*layers.IPv4)
	if !ok {
		return ErrInvalidArgument("handleICMP", ipv4Layer)
	}
	icmp, ok := icmpLayer.(*layers.ICMPv4)
	if !ok {
		return ErrInvalidArgument("handleICMP", icmpLayer)
	}
	payloadLayer := pkt.Layer(gopacket.LayerTypePayload)
	if payloadLayer == nil {
		return ErrInvalidArgument("handleICMP", pkt)
	}
	payload, ok := payloadLayer.(*gopacket.Payload)
	if !ok {
		return ErrInvalidArgument("handleICMP", payloadLayer)
	}

	if icmp.TypeCode.Type() != layers.ICMPv4TypeEchoRequest {
		log.Infoln("unsupported ICMP type:", icmp.TypeCode)
		return nil
	}

	ethResp := &layers.Ethernet{
		SrcMAC:       a.datapathMAC,
		DstMAC:       eth.SrcMAC,
		EthernetType: eth.EthernetType,
	}
	ipv4Resp := &layers.IPv4{
		Version: ipv4.Version,
		//IHL:        0,
		//Length:     0,
		Id:       0, // No ID on purpose
		Flags:    layers.IPv4DontFragment,
		TTL:      64,
		Protocol: ipv4.Protocol,
		SrcIP:    a.accessIP,
		DstIP:    ipv4.SrcIP,
		//Options:    nil,
		//Padding:    nil,
	}
	icmpResp := &layers.ICMPv4{
		TypeCode: layers.ICMPv4TypeEchoReply,
		Checksum: 0,
		Id:       icmp.Id,
		Seq:      icmp.Seq,
	}

	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, ethResp, ipv4Resp, icmpResp, payload)
	if err != nil {
		log.Error(err)
		log.Errorln(gopacket.LayerDump(ethResp))
		log.Errorln(gopacket.LayerDump(ipv4Resp))
		log.Errorln(gopacket.LayerDump(icmpResp))
		log.Errorln(gopacket.LayerDump(payload))
		return err
	}

	err = a.sendPacketOut(buf)
	if err != nil {
		return err
	}
	txPkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
	log.Tracef("Sent ICMP reply: %v", txPkt)

	return nil
}

func (a *aether) sessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) (err error) {
	// TODO: implement
	log.Traceln("sessionStats are not implemented in aether pipeline")
	return
}

// setupInterfaceClassification inserts the necessary interface classification rules.
func (a *aether) setupInterfaceClassification() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	calls := 0

	// Other GTP packets directly to UPF are uplink, from access.
	ifc := interfaceClassification{
		priority:    40,
		dstIp:       ip2int(a.accessIP),
		dstIpMask:   math.MaxUint32,
		ipProto:     uint8(UdpProto),
		ipProtoMask: math.MaxUint8,
		dstPort:     tunnelGTPUPort,
		dstPortMask: math.MaxUint16,

		gate:     0,
		srcIface: access,
	}
	if err = a.addInterfaceClassification(ctx, done, ifc); err != nil {
		log.Errorln(err)
		return
	}
	calls++

	ifc = interfaceClassification{
		priority:  30,
		dstIp:     ip2int(a.ueSubnet.IP),
		dstIpMask: ipMask2int(a.ueSubnet.Mask),

		gate:     0,
		srcIface: core,
	}
	if err = a.addInterfaceClassification(ctx, done, ifc); err != nil {
		log.Errorln(err)
		return
	}
	calls++

	// Other packets addressed to the UPF are packet ins.
	ifc = interfaceClassification{
		priority:  1,
		dstIp:     ip2int(a.accessIP),
		dstIpMask: math.MaxUint32,

		gate:     0,
		srcIface: 0,
	}
	if err = a.addInterfaceClassification(ctx, done, ifc); err != nil {
		log.Errorln(err)
		return
	}
	calls++

	rc := a.GRPCJoin(calls, Timeout, done)
	if !rc {
		return ErrOperationFailedWithReason("GRPCJoin", "Unable to make GRPC calls")
	}

	return
}

func (a *aether) addInterfaceClassification(ctx context.Context, done chan<- bool, ifc interfaceClassification) error {
	go func() {
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

		err := a.processInterfaceClassification(ctx, f, upfMsgTypeAdd)
		if err != nil {
			log.Errorln(err)
			return
		}

		done <- true
	}()

	return nil
}

func (a *aether) processInterfaceClassification(ctx context.Context, msg proto.Message, method upfMsgType) error {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		return ErrInvalidArgumentWithReason("method", method, "invalid method name")
	}

	any, err := anypb.New(msg)
	if err != nil {
		log.Println("Error marshalling the rule", msg, err)
		return err
	}

	log.Tracef("%+v", any)

	methods := [...]string{"add", "add", "delete", "clear"}

	resp, err := a.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "interfaceClassification",
		Cmd:  methods[method],
		Arg:  any,
	})

	if err != nil || resp.GetError() != nil {
		log.Errorf("interfaceClassification method failed with resp: %v, err: %v\n", resp, err)
	}
	return nil
}
