// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"google.golang.org/protobuf/proto"
	"math"
	"net"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	// DefaultBurstSize for cbs, pbs required for dpdk metering. 32 MTUs.
	DefaultBurstSize = 32 * 1514
	// SockAddr : Unix Socket path to read bess notification from.
	SockAddr = "/tmp/notifycp"
	// PfcpAddr : Unix Socket path to send end marker packet.
	PfcpAddr = "/tmp/pfcpport"
	// far-action specific values.
	farForwardD = 0x0
	farForwardU = 0x1
	farDrop     = 0x2
	farBuffer   = 0x3
	farNotify   = 0x4
)

const (
	// Internal gates for QER.
	qerGateMeter      uint64 = iota
	qerGateStatusDrop        = iota + 4
	qerGateUnmeter
)

const (
	// Internal gates for Slice meter.
	sliceMeterGateMeter      uint64 = iota
	sliceMeterGateLookupFail        = iota + 4
	sliceMeterGateUnmeter
)

var intEnc = func(u uint64) *pb.FieldData {
	return &pb.FieldData{Encoding: &pb.FieldData_ValueInt{ValueInt: u}}
}

var bessIP = flag.String("bess", "localhost:10514", "BESS IP/port combo")

type bess struct {
	client           pb.BESSControlClient
	conn             *grpc.ClientConn
	endMarkerSocket  net.Conn
	notifyBessSocket net.Conn
	endMarkerChan    chan []byte
	qciQosMap        map[uint8]*QosConfigVal
}

func (b *bess) setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn) {
}

func (b *bess) isConnected(accessIP *net.IP) bool {
	if (b.conn == nil) || (int(b.conn.GetState()) != Ready) {
		return false
	}

	return true
}

func (b *bess) sendEndMarkers(endMarkerList *[][]byte) error {
	for _, eMarker := range *endMarkerList {
		b.endMarkerChan <- eMarker
	}

	return nil
}

func (b *bess) addSliceInfo(sliceInfo *SliceInfo) error {
	var sliceMeterConfig SliceMeterConfig
	sliceMeterConfig.N6RateBps = sliceInfo.uplinkMbr
	sliceMeterConfig.N3RateBps = sliceInfo.downlinkMbr
	sliceMeterConfig.N6BurstBytes = sliceInfo.ulBurstBytes
	sliceMeterConfig.N3BurstBytes = sliceInfo.dlBurstBytes

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	done := make(chan bool)

	b.addSliceMeter(ctx, done, sliceMeterConfig)

	rc := b.GRPCJoin(1, Timeout, done)

	if !rc {
		log.Errorln("Unable to make GRPC calls")
	}

	return nil
}

func (b *bess) sendMsgToUPF(
	method upfMsgType, pdrs []pdr, fars []far, qers []qer, sessionQers []qer,
) uint8 {
	// create context
	var cause uint8 = ie.CauseRequestAccepted

	calls := len(pdrs) + len(fars) + len(qers) + len(sessionQers)
	if calls == 0 {
		return cause
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	done := make(chan bool)

	for _, pdr := range pdrs {
		log.Traceln(pdr)

		switch method {
		case upfMsgTypeAdd:
			fallthrough
		case upfMsgTypeMod:
			b.addPDR(ctx, done, pdr)
		case upfMsgTypeDel:
			b.delPDR(ctx, done, pdr)
		}
	}

	for _, far := range fars {
		log.Traceln(far)

		switch method {
		case upfMsgTypeAdd:
			fallthrough
		case upfMsgTypeMod:
			b.addFAR(ctx, done, far)
		case upfMsgTypeDel:
			b.delFAR(ctx, done, far)
		}
	}

	for _, qer := range qers {
		log.Traceln("Application qer:", qer)

		switch method {
		case upfMsgTypeAdd:
			fallthrough
		case upfMsgTypeMod:
			b.addApplicationQER(ctx, done, qer)
		case upfMsgTypeDel:
			b.delApplicationQER(ctx, done, qer)
		}
	}

	for _, qer := range sessionQers {
		log.Traceln("Session qer:", qer)

		switch method {
		case upfMsgTypeAdd:
			fallthrough
		case upfMsgTypeMod:
			b.addSessionQER(ctx, done, qer)
		case upfMsgTypeDel:
			b.delSessionQER(ctx, done, qer)
		}
	}

	rc := b.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}

	return cause
}

func (b *bess) sendDeleteAllSessionsMsgtoUPF() {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	done := make(chan bool)
	calls := 6

	b.removeAllPDRs(ctx, done)
	b.removeAllFARs(ctx, done)
	b.removeAllApplicationQERs(ctx, done)
	b.removeAllSessionQERs(ctx, done)
	b.removeAllCounters(ctx, done, "preQoSCounter")
	b.removeAllCounters(ctx, done, "postDLQoSCounter")
	b.removeAllCounters(ctx, done, "postULQoSCounter")

	rc := b.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}
}

func (b *bess) exit() {
	log.Println("Exit function Bess")
	b.conn.Close()
}

func (b *bess) measure(ifName string, f *pb.MeasureCommandGetSummaryArg) *pb.MeasureCommandGetSummaryResponse {
	modName := func() string {
		return ifName + "_measure"
	}

	any, err := anypb.New(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return nil
	}

	ctx := context.Background()

	modRes, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: modName(),
		Cmd:  "get_summary",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error calling get_summary on module", modName(), err)
		return nil
	}

	var res pb.MeasureCommandGetSummaryResponse

	err = modRes.GetData().UnmarshalTo(&res)
	if err != nil {
		log.Println("Error unmarshalling the response", modName(), err)
		return nil
	}

	return &res
}

func (b *bess) getPortStats(ifname string) *pb.GetPortStatsResponse {
	ctx := context.Background()
	req := &pb.GetPortStatsRequest{
		Name: ifname + "Fast",
	}

	res, err := b.client.GetPortStats(ctx, req)
	if err != nil || res.GetError() != nil {
		log.Println("Error calling GetPortStats", ifname, err, res.GetError().Errmsg)
		return nil
	}

	return res
}

func (b *bess) portStats(uc *upfCollector, ch chan<- prometheus.Metric) {
	portstats := func(ifaceLabel, ifaceName string) {
		packets := func(packets uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.packets,
				prometheus.CounterValue,
				float64(packets),
				ifaceLabel, direction,
			)
			ch <- p
		}
		bytes := func(bytes uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.bytes,
				prometheus.CounterValue,
				float64(bytes),
				ifaceLabel, direction,
			)
			ch <- p
		}
		dropped := func(dropped uint64, direction string) {
			p := prometheus.MustNewConstMetric(
				uc.dropped,
				prometheus.CounterValue,
				float64(dropped),
				ifaceLabel, direction,
			)
			ch <- p
		}

		res := b.getPortStats(ifaceName)
		if res == nil {
			return
		}

		packets(res.Inc.Packets, "rx")
		packets(res.Out.Packets, "tx")

		bytes(res.Inc.Bytes, "rx")
		bytes(res.Out.Bytes, "tx")

		dropped(res.Inc.Dropped, "rx")
		dropped(res.Out.Dropped, "tx")
	}

	portstats("Access", uc.upf.accessIface)
	portstats("Core", uc.upf.coreIface)
}

func (b *bess) summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric) {
	measureIface := func(ifaceLabel, ifaceName string) {
		req := &pb.MeasureCommandGetSummaryArg{
			Clear:              true,
			LatencyPercentiles: getPctiles(),
			JitterPercentiles:  getPctiles(),
		}

		res := b.measure(ifaceName, req)
		if res == nil {
			return
		}

		latencies := res.GetLatency().GetPercentileValuesNs()
		if latencies != nil {
			l := prometheus.MustNewConstSummary(
				uc.latency,
				res.Packets,
				float64(res.Latency.GetTotalNs()),
				makeBuckets(latencies),
				ifaceLabel,
			)

			ch <- l
		}

		jitters := res.GetJitter().GetPercentileValuesNs()
		if jitters != nil {
			j := prometheus.MustNewConstSummary(
				uc.jitter,
				res.Packets,
				float64(res.Jitter.GetTotalNs()),
				makeBuckets(jitters),
				ifaceLabel,
			)

			ch <- j
		}
	}
	measureIface("Access", uc.upf.accessIface)
	measureIface("Core", uc.upf.coreIface)
}

func (b *bess) endMarkerSendLoop(endMarkerChan chan []byte) {
	for outPacket := range endMarkerChan {
		_, err := b.endMarkerSocket.Write(outPacket)
		if err != nil {
			log.Println("end marker write failed")
		}
	}
}

func (b *bess) notifyListen(reportNotifyChan chan<- uint64) {
	for {
		buf := make([]byte, 512)

		_, err := b.notifyBessSocket.Read(buf)
		if err != nil {
			return
		}

		d := buf[0:8]
		fseid := binary.LittleEndian.Uint64(d)
		reportNotifyChan <- fseid
	}
}

func (b *bess) readQciQosMap(conf *Conf) {
	b.qciQosMap = make(map[uint8]*QosConfigVal)

	for _, qosVal := range conf.QciQosConfig {
		qosConfigVal := &QosConfigVal{
			cbs:              qosVal.CBS,
			ebs:              qosVal.EBS,
			pbs:              qosVal.PBS,
			burstDurationMs:  qosVal.BurstDurationMs,
			schedulePriority: qosVal.SchedulingPriority,
		}
		b.qciQosMap[qosVal.QCI] = qosConfigVal
	}

	if _, ok := b.qciQosMap[0]; !ok {
		b.qciQosMap[0] = &QosConfigVal{
			cbs:              DefaultBurstSize,
			ebs:              DefaultBurstSize,
			pbs:              DefaultBurstSize,
			burstDurationMs:  10,
			schedulePriority: 7,
		}
	}
}

func (b *bess) setUpfInfo(u *upf, conf *Conf) {
	log.Println("setUpfInfo bess")

	u.simInfo = &conf.SimInfo
	u.ippoolCidr = conf.CPIface.UeIPPool

	log.Println("IP pool : ", u.ippoolCidr)

	errin := u.ippool.initPool(u.ippoolCidr)
	if errin != nil {
		log.Println("ip pool init failed")
	}

	u.accessIP = ParseIP(conf.AccessIface.IfName, "Access")
	u.coreIP = ParseIP(conf.CoreIface.IfName, "Core")

	b.readQciQosMap(conf)
	// get bess grpc client
	log.Println("bessIP ", *bessIP)

	b.endMarkerChan = make(chan []byte, 1024)

	b.conn, errin = grpc.Dial(*bessIP, grpc.WithInsecure())
	if errin != nil {
		log.Fatalln("did not connect:", errin)
	}

	b.client = pb.NewBESSControlClient(b.conn)

	if conf.EnableNotifyBess {
		notifySockAddr := conf.NotifySockAddr
		if notifySockAddr == "" {
			notifySockAddr = SockAddr
		}

		b.notifyBessSocket, errin = net.Dial("unixpacket", notifySockAddr)
		if errin != nil {
			log.Println("dial error:", errin)
			return
		}

		go b.notifyListen(u.reportNotifyChan)
	}

	if conf.EnableEndMarker {
		pfcpCommAddr := conf.EndMarkerSockAddr
		if pfcpCommAddr == "" {
			pfcpCommAddr = PfcpAddr
		}

		b.endMarkerSocket, errin = net.Dial("unixpacket", pfcpCommAddr)
		if errin != nil {
			log.Println("dial error:", errin)
			return
		}

		log.Println("Starting end marker loop")

		go b.endMarkerSendLoop(b.endMarkerChan)
	}

	if (conf.SliceMeterConfig.N6RateBps > 0) ||
		(conf.SliceMeterConfig.N3RateBps > 0) {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()

		done := make(chan bool)

		b.addSliceMeter(ctx, done, conf.SliceMeterConfig)

		rc := b.GRPCJoin(1, Timeout, done)
		if !rc {
			log.Errorln("Unable to make GRPC calls")
		}
	}
}

func (b *bess) sim(u *upf, method string) {
	start := time.Now()
	// const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	ueip := u.simInfo.StartUEIP
	enbip := u.simInfo.StartENBIP
	aupfip := u.simInfo.StartAUPFIP
	n9appip := u.simInfo.N9AppIP
	n3TEID := hex2int(u.simInfo.StartN3TEID)
	n9TEID := hex2int(u.simInfo.StartN9TEID)

	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80

	for i := uint32(0); i < u.maxSessions; i++ {
		// NG4T-based formula to calculate enodeB IP address against a given UE IP address
		// il_trafficgen also uses the same scheme
		// See SimuCPEnbv4Teid(...) in ngic code for more details
		ueOfRan := i % ng4tMaxUeRan
		ran := i / ng4tMaxUeRan
		enbOfRan := ueOfRan % ng4tMaxEnbRan
		enbIdx := ran*ng4tMaxEnbRan + enbOfRan

		// create/delete downlink pdr
		pdrN6Down := pdr{
			srcIface: core,
			dstIP:    ip2int(ueip) + i,

			srcIfaceMask: 0xFF,
			dstIPMask:    0xFFFFFFFF,

			precedence: 255,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n3,
			qerID:     n6,
			needDecap: 0,
		}

		pdrN9Down := pdr{
			srcIface:     core,
			tunnelTEID:   n9TEID + i,
			tunnelIP4Dst: ip2int(u.coreIP),

			srcIfaceMask:     0xFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			tunnelIP4DstMask: 0xFFFFFFFF,

			precedence: 1,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n3,
			qerID:     n9,
			needDecap: 1,
		}

		// create/delete uplink pdr
		pdrN6Up := pdr{
			srcIface:     access,
			tunnelIP4Dst: ip2int(u.accessIP),
			tunnelTEID:   n3TEID + i,
			srcIP:        ip2int(ueip) + i,

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			srcIPMask:        0xFFFFFFFF,

			precedence: 255,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n6,
			qerID:     n6,
			needDecap: 1,
		}

		pdrN9Up := pdr{
			srcIface:     access,
			tunnelIP4Dst: ip2int(u.accessIP),
			tunnelTEID:   n3TEID + i,
			dstIP:        ip2int(n9appip),

			srcIfaceMask:     0xFF,
			tunnelIP4DstMask: 0xFFFFFFFF,
			tunnelTEIDMask:   0xFFFFFFFF,
			dstIPMask:        0xFFFFFFFF,

			precedence: 1,

			fseID:     uint64(n3TEID + i),
			ctrID:     i,
			farID:     n9,
			qerID:     n9,
			needDecap: 1,
		}

		pdrs := []pdr{pdrN6Down, pdrN9Down, pdrN6Up, pdrN9Up}

		// create/delete downlink far
		farDown := far{
			farID: n3,
			fseID: uint64(n3TEID + i),

			applyAction:  ActionForward,
			dstIntf:      ie.DstInterfaceAccess,
			tunnelType:   0x1,
			tunnelIP4Src: ip2int(u.accessIP),
			tunnelIP4Dst: ip2int(enbip) + enbIdx,
			tunnelTEID:   n3TEID + i,
			tunnelPort:   tunnelGTPUPort,
		}

		// create/delete uplink far
		farN6Up := far{
			farID: n6,
			fseID: uint64(n3TEID + i),

			applyAction: ActionForward,
			dstIntf:     ie.DstInterfaceCore,
		}

		farN9Up := far{
			farID: n9,
			fseID: uint64(n3TEID + i),

			applyAction:  ActionForward,
			dstIntf:      ie.DstInterfaceCore,
			tunnelType:   0x1,
			tunnelIP4Src: ip2int(u.coreIP),
			tunnelIP4Dst: ip2int(aupfip),
			tunnelTEID:   n9TEID + i,
			tunnelPort:   tunnelGTPUPort,
		}

		fars := []far{farDown, farN6Up, farN9Up}

		// create/delete uplink qer
		qerN6 := qer{
			qerID: n6,
			fseID: uint64(n3TEID + i),
			qfi:   9,
			ulGbr: 50000,
			ulMbr: 90000,
			dlGbr: 60000,
			dlMbr: 80000,
		}

		qerN9 := qer{
			qerID: n9,
			fseID: uint64(n3TEID + i),
			qfi:   8,
			ulGbr: 50000,
			ulMbr: 60000,
			dlGbr: 70000,
			dlMbr: 90000,
		}

		qers := []qer{qerN6, qerN9}

		// create/delete session qers
		sessionQer := qer{
			qerID: 0,
			fseID: uint64(n3TEID + i),
			qfi:   0,
			ulGbr: 0,
			ulMbr: 1000,
			dlGbr: 0,
			dlMbr: 1000,
		}
		sessionQers := []qer{sessionQer}

		switch method {
		case "create":
			b.sendMsgToUPF(upfMsgTypeAdd, pdrs, fars, qers, sessionQers)

		case "delete":
			b.sendMsgToUPF(upfMsgTypeDel, pdrs, fars, qers, sessionQers)

		default:
			log.Fatalln("Unsupported method", method)
		}
	}
	log.Println("Sessions/s:", float64(u.maxSessions)/time.Since(start).Seconds())
}

func (b *bess) processPDR(ctx context.Context, any *anypb.Any, method upfMsgType) {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		log.Println("Invalid method name: ", method)
		return
	}

	methods := [...]string{"add", "add", "delete", "clear"}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "pdrLookup",
		Cmd:  methods[method],
		Arg:  any,
	})
	if err != nil {
		log.Println("pdrLookup method failed!:", err)
	}
}

func (b *bess) addPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.WildcardMatchCommandAddArg{
			Gate:     uint64(p.needDecap),
			Priority: int64(math.MaxUint32 - p.precedence),
			Values: []*pb.FieldData{
				intEnc(uint64(p.srcIface)),     /* src_iface */
				intEnc(uint64(p.tunnelIP4Dst)), /* tunnel_ipv4_dst */
				intEnc(uint64(p.tunnelTEID)),   /* enb_teid */
				intEnc(uint64(p.srcIP)),        /* ueaddr ip*/
				intEnc(uint64(p.dstIP)),        /* inet ip */
				intEnc(uint64(p.srcPort)),      /* ue port */
				intEnc(uint64(p.dstPort)),      /* inet port */
				intEnc(uint64(p.proto)),        /* proto id */
			},
			Masks: []*pb.FieldData{
				intEnc(uint64(p.srcIfaceMask)),     /* src_iface-mask */
				intEnc(uint64(p.tunnelIP4DstMask)), /* tunnel_ipv4_dst-mask */
				intEnc(uint64(p.tunnelTEIDMask)),   /* enb_teid-mask */
				intEnc(uint64(p.srcIPMask)),        /* ueaddr ip-mask */
				intEnc(uint64(p.dstIPMask)),        /* inet ip-mask */
				intEnc(uint64(p.srcPortMask)),      /* ue port-mask */
				intEnc(uint64(p.dstPortMask)),      /* inet port-mask */
				intEnc(uint64(p.protoMask)),        /* proto id-mask */
			},
			Valuesv: []*pb.FieldData{
				intEnc(uint64(p.pdrID)), /* pdr-id */
				intEnc(p.fseID),         /* fseid */
				intEnc(uint64(p.ctrID)), /* ctr_id */
				intEnc(uint64(p.qerID)), /* qer_id */
				intEnc(uint64(p.farID)), /* far_id */
			},
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, upfMsgTypeAdd)
		done <- true
	}()
}

func (b *bess) delPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.WildcardMatchCommandDeleteArg{
			Values: []*pb.FieldData{
				intEnc(uint64(p.srcIface)),     /* src_iface */
				intEnc(uint64(p.tunnelIP4Dst)), /* tunnel_ipv4_dst */
				intEnc(uint64(p.tunnelTEID)),   /* enb_teid */
				intEnc(uint64(p.srcIP)),        /* ueaddr ip*/
				intEnc(uint64(p.dstIP)),        /* inet ip */
				intEnc(uint64(p.srcPort)),      /* ue port */
				intEnc(uint64(p.dstPort)),      /* inet port */
				intEnc(uint64(p.proto)),        /* proto id */
			},
			Masks: []*pb.FieldData{
				intEnc(uint64(p.srcIfaceMask)),     /* src_iface-mask */
				intEnc(uint64(p.tunnelIP4DstMask)), /* tunnel_ipv4_dst-mask */
				intEnc(uint64(p.tunnelTEIDMask)),   /* enb_teid-mask */
				intEnc(uint64(p.srcIPMask)),        /* ueaddr ip-mask */
				intEnc(uint64(p.dstIPMask)),        /* inet ip-mask */
				intEnc(uint64(p.srcPortMask)),      /* ue port-mask */
				intEnc(uint64(p.dstPortMask)),      /* inet port-mask */
				intEnc(uint64(p.protoMask)),        /* proto id-mask */
			},
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Errorln("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, upfMsgTypeDel)
		done <- true
	}()
}

func (b *bess) processApplicationQER(ctx context.Context, any *anypb.Any, method upfMsgType) {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		log.Errorln("Invalid method name: ", method)
		return
	}

	methods := [...]string{"add", "add", "delete", "clear"}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "appQERLookup",
		Cmd:  methods[method],
		Arg:  any,
	})
	if err != nil {
		log.Errorf("appQERLookup %v for qer %v failed with error: %v", methods[method], any, err)
	}
}

func (b *bess) addApplicationQER(ctx context.Context, done chan<- bool, qer qer) {
	go func() {
		var (
			any                           *anypb.Any
			err                           error
			cir, pir, cbs, ebs, pbs, gate uint64
			srcIface                      uint8
		)

		// Uplink QER
		srcIface = access

		// Lookup QCI from QFI, else try default QCI.
		qosVal, ok := b.qciQosMap[qer.qfi]
		if !ok {
			log.Debug("No config for qfi/qci : ", qer.qfi, ". Using default burst size.")
			qosVal = b.qciQosMap[0]
		}
		cbs = maxUint64(calcBurstSizeFromRate(qer.ulGbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.cbs))
		ebs = maxUint64(calcBurstSizeFromRate(qer.ulMbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.ebs))
		pbs = maxUint64(calcBurstSizeFromRate(qer.ulMbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.ebs))

		if qer.ulStatus == ie.GateStatusClosed {
			gate = qerGateStatusDrop
		} else if qer.ulMbr != 0 || qer.ulGbr != 0 {
			/* MBR/GBR is received in Kilobits/sec.
			   CIR/PIR is sent in bytes */
			cir = maxUint64(((qer.ulGbr * 1000) / 8), 1)
			pir = maxUint64(((qer.ulMbr * 1000) / 8), cir)
			gate = qerGateMeter
		} else {
			gate = qerGateUnmeter
		}

		q := &pb.QosCommandAddArg{
			Gate: gate,
			Cir:  cir, /* committed info rate */
			Pir:  pir, /* peak info rate */
			Cbs:  cbs, /* committed burst size */
			Pbs:  pbs, /* Peak burst size */
			Ebs:  ebs, /* Excess burst size */
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)),  /* Src Intf */
				intEnc(uint64(qer.qerID)), /* qer_id */
				intEnc(qer.fseID),         /* fseid */
			},
			Values: []*pb.FieldData{
				intEnc(uint64(qer.qfi)), /* QFI */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Errorln("Error marshalling the rule", q, err)
			return
		}

		b.processApplicationQER(ctx, any, upfMsgTypeAdd)

		// Downlink QER
		srcIface = core

		// Lookup QCI from QFI, else try default QCI.
		qosVal, ok = b.qciQosMap[qer.qfi]
		if !ok {
			log.Debug("No config for qfi/qci : ", qer.qfi, ". Using default burst size.")
			qosVal = b.qciQosMap[0]
		}
		cbs = maxUint64(calcBurstSizeFromRate(qer.dlGbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.cbs))
		ebs = maxUint64(calcBurstSizeFromRate(qer.dlMbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.ebs))
		pbs = maxUint64(calcBurstSizeFromRate(qer.dlMbr, uint64(qosVal.burstDurationMs)), uint64(qosVal.ebs))

		if qer.dlStatus == ie.GateStatusClosed {
			gate = qerGateStatusDrop
		} else if qer.dlMbr != 0 || qer.dlGbr != 0 {
			/* MBR/GBR is received in Kilobits/sec.
			   CIR/PIR is sent in bytes */
			cir = maxUint64(((qer.dlGbr * 1000) / 8), 1)
			pir = maxUint64(((qer.dlMbr * 1000) / 8), cir)
			gate = qerGateMeter
		} else {
			gate = qerGateUnmeter
		}

		q = &pb.QosCommandAddArg{
			Gate: gate,
			Cir:  cir, /* committed info rate */
			Pir:  pir, /* peak info rate */
			Cbs:  cbs, /* committed burst size */
			Pbs:  pbs, /* Peak burst size */
			Ebs:  ebs, /* Excess burst size */
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)),  /* Src Intf */
				intEnc(uint64(qer.qerID)), /* qer_id */
				intEnc(qer.fseID),         /* fseid */
			},
			Values: []*pb.FieldData{
				intEnc(uint64(qer.qfi)), /* QFI */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Errorln("Error marshalling the rule", q, err)
			return
		}

		b.processApplicationQER(ctx, any, upfMsgTypeAdd)
		done <- true
	}()
}

func (b *bess) delApplicationQER(ctx context.Context, done chan<- bool, qer qer) {
	go func() {
		var (
			any      *anypb.Any
			err      error
			srcIface uint8
		)

		// Uplink QER
		srcIface = access

		q := &pb.QosCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)),  /* Src Intf */
				intEnc(uint64(qer.qerID)), /* qer_id */
				intEnc(qer.fseID),         /* fseid */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Println("Error marshalling the rule", q, err)
			return
		}

		b.processApplicationQER(ctx, any, upfMsgTypeDel)

		// Downlink QER
		srcIface = core

		q = &pb.QosCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)),  /* Src Intf */
				intEnc(uint64(qer.qerID)), /* qer_id */
				intEnc(qer.fseID),         /* fseid */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Println("Error marshalling the rule", q, err)
			return
		}

		b.processApplicationQER(ctx, any, upfMsgTypeDel)
		done <- true
	}()
}

func (b *bess) processFAR(ctx context.Context, any *anypb.Any, method upfMsgType) {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		log.Println("Invalid method name: ", method)
		return
	}

	methods := [...]string{"add", "add", "delete", "clear"}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "farLookup",
		Cmd:  methods[method],
		Arg:  any,
	})
	if err != nil {
		log.Println("farLookup method failed!:", err)
	}
}

func (b *bess) setActionValue(f far) uint8 {
	if (f.applyAction & ActionForward) != 0 {
		if f.dstIntf == ie.DstInterfaceAccess {
			return farForwardD
		} else if (f.dstIntf == ie.DstInterfaceCore) || (f.dstIntf == ie.DstInterfaceSGiLANN6LAN) {
			return farForwardU
		}
	} else if (f.applyAction & ActionDrop) != 0 {
		return farDrop
	} else if (f.applyAction & ActionBuffer) != 0 {
		return farNotify
	} else if (f.applyAction & ActionNotify) != 0 {
		return farNotify
	}

	// default action
	return farDrop
}

func (b *bess) addFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		action := b.setActionValue(far)
		f := &pb.ExactMatchCommandAddArg{
			Gate: uint64(far.tunnelType),
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(far.fseID),         /* fseid */
			},
			Values: []*pb.FieldData{
				intEnc(uint64(action)),           /* action */
				intEnc(uint64(far.tunnelType)),   /* tunnel_out_type */
				intEnc(uint64(far.tunnelIP4Src)), /* access-ip */
				intEnc(uint64(far.tunnelIP4Dst)), /* enb ip */
				intEnc(uint64(far.tunnelTEID)),   /* enb teid */
				intEnc(uint64(far.tunnelPort)),   /* udp gtpu port */
			},
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processFAR(ctx, any, upfMsgTypeAdd)
		done <- true
	}()
}

func (b *bess) delFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.ExactMatchCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(far.fseID),         /* fseid */
			},
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processFAR(ctx, any, upfMsgTypeDel)
		done <- true
	}()
}

func (b *bess) processCounters(ctx context.Context, any *anypb.Any, method upfMsgType, counterName string) {
	methods := [...]string{"add", "add", "remove", "removeAll"}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: counterName,
		Cmd:  methods[method],
		Arg:  any,
	})
	if err != nil {
		log.Println("counter method failed!:", err)
	}
}

func (b *bess) addCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.CounterAddArg{
			CtrId: ctrID,
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processCounters(ctx, any, upfMsgTypeAdd, counterName)
		done <- true
	}()
}

func (b *bess) delCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.CounterRemoveArg{
			CtrId: ctrID,
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processCounters(ctx, any, upfMsgTypeDel, counterName)
		done <- true
	}()
}

func (b *bess) processSliceMeter(ctx context.Context, any *anypb.Any, method upfMsgType) {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		log.Errorln("Invalid method name: ", method)
		return
	}

	methods := [...]string{"add", "add", "delete", "clear"}

	_, err := b.client.ModuleCommand(
		ctx, &pb.CommandRequest{
			Name: "sliceMeter",
			Cmd:  methods[method],
			Arg:  any,
		},
	)
	if err != nil {
		log.Errorln("sliceMeter method failed!:", err)
	}
}

func (b *bess) addSliceMeter(ctx context.Context, done chan<- bool, meterConfig SliceMeterConfig) {
	go func() {
		var (
			any                           *anypb.Any
			err                           error
			cir, pir, cbs, ebs, pbs, gate uint64
		)

		// Uplink N6 slice meter config
		if meterConfig.N6RateBps != 0 {
			gate = sliceMeterGateMeter
			cir = 1                         // Mark all traffic as yellow
			pir = meterConfig.N6RateBps / 8 // bit/s to byte/s
		} else {
			gate = sliceMeterGateUnmeter
		}

		if meterConfig.N6BurstBytes != 0 {
			cbs = 1 // Mark all traffic as yellow
			pbs = meterConfig.N6BurstBytes
			ebs = 0 // Unused
		} else {
			cbs = 1 // Mark all traffic as yellow
			pbs = DefaultBurstSize
			ebs = 0 // Unused
		}

		q := &pb.QosCommandAddArg{
			Gate:              gate,
			Cir:               cir,                               /* committed info rate */
			Pir:               pir,                               /* peak info rate */
			Cbs:               cbs,                               /* committed burst size */
			Pbs:               pbs,                               /* Peak burst size */
			Ebs:               ebs,                               /* Excess burst size */
			OptionalDeductLen: &pb.QosCommandAddArg_DeductLen{0}, /* Include all headers */
			Fields: []*pb.FieldData{
				intEnc(uint64(farForwardU)), /* Action */
				intEnc(uint64(0)),           /* tunnel_out_type */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Errorln("Error marshalling the rule", q, err)
			return
		}

		b.processSliceMeter(ctx, any, upfMsgTypeAdd)

		// Downlink N3 slice meter config
		if meterConfig.N3RateBps != 0 {
			gate = sliceMeterGateMeter
			cir = 1                         // Mark all traffic as yellow
			pir = meterConfig.N3RateBps / 8 // bit/s to byte/s
		} else {
			gate = sliceMeterGateUnmeter
		}

		if meterConfig.N3BurstBytes != 0 {
			cbs = 1 // Mark all traffic as yellow
			pbs = meterConfig.N3BurstBytes
			ebs = 0 // Unused
		} else {
			cbs = 1 // Mark all traffic as yellow
			pbs = DefaultBurstSize
			ebs = 0 // Unused
		}
		// TODO: packet deduction should take GTPU extension header into account
		q = &pb.QosCommandAddArg{
			Gate:              gate,
			Cir:               cir,                                /* committed info rate */
			Pir:               pir,                                /* peak info rate */
			Cbs:               cbs,                                /* committed burst size */
			Pbs:               pbs,                                /* Peak burst size */
			Ebs:               ebs,                                /* Excess burst size */
			OptionalDeductLen: &pb.QosCommandAddArg_DeductLen{50}, /* Exclude Ethernet,IP,UDP,GTP header */
			Fields: []*pb.FieldData{
				intEnc(uint64(farForwardD)), /* Action */
				intEnc(uint64(1)),           /* tunnel_out_type */
			},
		}

		any, err = anypb.New(q)
		if err != nil {
			log.Errorln("Error marshalling the rule", q, err)
			return
		}

		b.processSliceMeter(ctx, any, upfMsgTypeAdd)
		done <- true
	}()
}

func (b *bess) processSessionQER(ctx context.Context, proto proto.Message, method upfMsgType) error {
	if method != upfMsgTypeAdd && method != upfMsgTypeDel && method != upfMsgTypeClear {
		return fmt.Errorf("invalid method name: %v", method)
	}

	methods := [...]string{"add", "add", "delete", "clear"}

	any, err := anypb.New(proto)
	if err != nil {
		return err
	}

	_, err = b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "sessionQERLookup",
		Cmd:  methods[method],
		Arg:  any,
	})
	if err != nil {
		log.Errorf("sessionQERLookup %v for qer %v failed with error: %v", methods[method], proto, err)
		return err
	}
	return nil
}

func (b *bess) addSessionQER(ctx context.Context, done chan<- bool, qer qer) {
	go func() {
		var (
			cir, pir, cbs, ebs, pbs, gate uint64
			srcIface                      uint8
		)

		if qosVal, ok := b.qciQosMap[qer.qfi]; ok {
			cbs = uint64(qosVal.cbs)
			ebs = uint64(qosVal.ebs)
			pbs = uint64(qosVal.pbs)
		} else {
			log.Println("No config for qfi/qci : ", qer.qfi,
				". Using default burst size.")
			cbs = uint64(DefaultBurstSize)
			ebs = uint64(DefaultBurstSize)
			pbs = uint64(DefaultBurstSize)
		}

		// Uplink session QER
		srcIface = access

		if qer.ulStatus == ie.GateStatusClosed {
			gate = qerGateStatusDrop
		} else if qer.ulMbr != 0 || qer.ulGbr != 0 {
			/* MBR/GBR is received in Kilobits/sec.
			   CIR/PIR is sent in bytes */
			cir = maxUint64((qer.ulGbr*1000)/8, 1)
			pir = maxUint64((qer.ulMbr*1000)/8, cir)
			gate = qerGateMeter
		} else {
			gate = qerGateUnmeter
		}

		q := &pb.QosCommandAddArg{
			Gate: gate,
			Cir:  cir, /* committed info rate */
			Pir:  pir, /* peak info rate */
			Cbs:  cbs, /* committed burst size */
			Pbs:  pbs, /* Peak burst size */
			Ebs:  ebs, /* Excess burst size */
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)), /* Src Intf */
				intEnc(qer.fseID),        /* fseid */
			},
		}

		if err := b.processSessionQER(ctx, q, upfMsgTypeAdd); err != nil {
			log.Errorln("Error adding uplink session qer %v:", q, err)
		}

		// Downlink session QER
		srcIface = core

		if qer.dlStatus == ie.GateStatusClosed {
			gate = qerGateStatusDrop
		} else if qer.dlMbr != 0 || qer.dlGbr != 0 {
			/* MBR/GBR is received in Kilobits/sec.
			   CIR/PIR is sent in bytes */
			cir = maxUint64((qer.dlGbr*1000)/8, 1)
			pir = maxUint64((qer.dlMbr*1000)/8, cir)
			gate = qerGateMeter
		} else {
			gate = qerGateUnmeter
		}

		q = &pb.QosCommandAddArg{
			Gate: gate,
			Cir:  cir, /* committed info rate */
			Pir:  pir, /* peak info rate */
			Cbs:  cbs, /* committed burst size */
			Pbs:  pbs, /* Peak burst size */
			Ebs:  ebs, /* Excess burst size */
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)), /* Src Intf */
				intEnc(qer.fseID),        /* fseid */
			},
		}

		if err := b.processSessionQER(ctx, q, upfMsgTypeAdd); err != nil {
			log.Errorln("Error adding downlink session qer %v:", q, err)
		}

		done <- true
	}()
}

func (b *bess) delSessionQER(ctx context.Context, done chan<- bool, qer qer) {
	go func() {
		var (
			srcIface uint8
		)

		// Uplink session QER
		srcIface = access

		q := &pb.QosCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)), /* Src Intf */
				intEnc(qer.fseID),        /* fseid */
			},
		}

		if err := b.processSessionQER(ctx, q, upfMsgTypeDel); err != nil {
			log.Errorln("Error deleting uplink session qer %v:", q, err)
		}

		// Downlink session QER
		srcIface = core

		q = &pb.QosCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(srcIface)), /* Src Intf */
				intEnc(qer.fseID),        /* fseid */
			},
		}

		if err := b.processSessionQER(ctx, q, upfMsgTypeDel); err != nil {
			log.Errorln("Error deleting downlink session qer %v:", q, err)
		}

		done <- true
	}()
}

func (b *bess) removeAllPDRs(ctx context.Context, done chan<- bool) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.EmptyArg{}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, upfMsgTypeClear)
		done <- true
	}()
}

func (b *bess) removeAllFARs(ctx context.Context, done chan<- bool) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.EmptyArg{}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processFAR(ctx, any, upfMsgTypeClear)
		done <- true
	}()
}

func (b *bess) removeAllApplicationQERs(ctx context.Context, done chan<- bool) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.EmptyArg{}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processApplicationQER(ctx, any, upfMsgTypeClear)
		done <- true
	}()
}

func (b *bess) removeAllSessionQERs(ctx context.Context, done chan<- bool) {
	go func() {
		f := &pb.EmptyArg{}
		if err := b.processSessionQER(ctx, f, upfMsgTypeClear); err != nil {
			log.Errorln("Error clearing session qers:", err)
		}

		done <- true
	}()
}

func (b *bess) removeAllCounters(ctx context.Context, done chan<- bool, name string) {
	go func() {
		var (
			any *anypb.Any
			err error
		)

		f := &pb.EmptyArg{}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processCounters(ctx, any, upfMsgTypeClear, name)
		done <- true
	}()
}

func (b *bess) GRPCJoin(calls int, timeout time.Duration, done chan bool) bool {
	boom := time.After(timeout)

	for {
		select {
		case ok := <-done:
			if !ok {
				log.Println("Error making GRPC calls")
				return false
			}

			calls--
			if calls == 0 {
				return true
			}
		case <-boom:
			log.Println("Timed out adding entries")
			return false
		}
	}
}
