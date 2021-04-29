// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"flag"
	"log"
	"math"
	"net"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

var intEnc = func(u uint64) *pb.FieldData {
	return &pb.FieldData{Encoding: &pb.FieldData_ValueInt{ValueInt: u}}
}

var (
	bessIP = flag.String("bess", "localhost:10514", "BESS IP/port combo")
)

type bess struct {
	client pb.BESSControlClient
	conn   *grpc.ClientConn
}

func (b *bess) setInfo(udpConn *net.UDPConn, udpAddr net.Addr, pconn *PFCPConn) {
	log.Println("bess setUdpConn not handled")
}

func (b *bess) isConnected(accessIP *net.IP) bool {
	if (b.conn == nil) || (int(b.conn.GetState()) != Ready) {
		return false
	}

	return true
}

func (b *bess) sendMsgToUPF(method string, pdrs []pdr, fars []far) uint8 {
	// create context
	var cause uint8 = ie.CauseRequestAccepted
	calls := len(pdrs) + len(fars)
	if calls == 0 {
		return cause
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)

	log.Println("upf : ", b.client)
	log.Println("conn : ", b.conn)
	for _, pdr := range pdrs {
		switch method {
		case "add":
			fallthrough
		case "mod":
			b.addPDR(ctx, done, pdr)
		case "del":
			b.delPDR(ctx, done, pdr)
		}
	}
	for _, far := range fars {
		switch method {
		case "add":
			fallthrough
		case "mod":
			b.addFAR(ctx, done, far)
		case "del":
			b.delFAR(ctx, done, far)
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
	calls := 5
	b.removeAllPDRs(ctx, done)
	b.removeAllFARs(ctx, done)
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
	req := &pb.GetPortStatsRequest{
		Name: ifname + "Fast",
	}
	ctx := context.Background()
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

	// get bess grpc client
	log.Println("bessIP ", *bessIP)

	b.conn, errin = grpc.Dial(*bessIP, grpc.WithInsecure())
	if errin != nil {
		log.Fatalln("did not connect:", errin)
	}

	b.client = pb.NewBESSControlClient(b.conn)
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

			fseID:     n3TEID + i,
			ctrID:     i,
			farID:     n3,
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

			fseID:     n3TEID + i,
			ctrID:     i,
			farID:     n3,
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

			fseID:     n3TEID + i,
			ctrID:     i,
			farID:     n6,
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

			fseID:     n3TEID + i,
			ctrID:     i,
			farID:     n9,
			needDecap: 1,
		}

		pdrs := []pdr{pdrN6Down, pdrN9Down, pdrN6Up, pdrN9Up}

		// create/delete downlink far
		farDown := far{
			farID: n3,
			fseID: n3TEID + i,

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
			fseID: n3TEID + i,

			applyAction: ActionForward,
			dstIntf:     ie.DstInterfaceCore,
		}

		farN9Up := far{
			farID: n9,
			fseID: n3TEID + i,

			applyAction:  ActionForward,
			dstIntf:      ie.DstInterfaceCore,
			tunnelType:   0x1,
			tunnelIP4Src: ip2int(u.coreIP),
			tunnelIP4Dst: ip2int(aupfip),
			tunnelTEID:   n9TEID + i,
			tunnelPort:   tunnelGTPUPort,
		}

		fars := []far{farDown, farN6Up, farN9Up}

		switch timeout := 100 * time.Millisecond; method {
		case "create":
			b.simcreateEntries(pdrs, fars, timeout)

		case "delete":
			b.simdeleteEntries(pdrs, fars, timeout)

		default:
			log.Fatalln("Unsupported method", method)
		}
	}
	log.Println("Sessions/s:", float64(u.maxSessions)/time.Since(start).Seconds())
}

func (b *bess) simcreateEntries(pdrs []pdr, fars []far, timeout time.Duration) {
	calls := len(pdrs) + len(fars)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool)
	for _, pdrv := range pdrs {
		b.addPDR(ctx, done, pdrv)
	}

	for _, farv := range fars {
		b.addFAR(ctx, done, farv)
	}

	rc := b.GRPCJoin(calls, timeout, done)
	if !rc {
		log.Println("Unable to complete GRPC call(s). Deleting")
		go b.simdeleteEntries(pdrs, fars, timeout)
	}
}

func (b *bess) simdeleteEntries(pdrs []pdr, fars []far, timeout time.Duration) {
	calls := len(pdrs) + len(fars)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool)
	for _, pdrv := range pdrs {
		b.delPDR(ctx, done, pdrv)
	}

	for _, farv := range fars {
		b.delFAR(ctx, done, farv)
	}

	rc := b.GRPCJoin(calls, timeout, done)
	if !rc {
		log.Println("Unable to complete GRPC call(s)")
	}
}

func (b *bess) processPDR(ctx context.Context, any *anypb.Any, method string) {
	if method != "add" && method != "delete" && method != "clear" {
		log.Println("Invalid method name: ", method)
		return
	}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "pdrLookup",
		Cmd:  method,
		Arg:  any,
	})
	if err != nil {
		log.Println("pdrLookup method failed!:", err)
	}
}

func (b *bess) addPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var any *anypb.Any
		var err error

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
				intEnc(uint64(p.fseID)), /* fseid */
				intEnc(uint64(p.ctrID)), /* ctr_id */
				intEnc(uint64(p.farID)), /* far_id */
			},
		}
		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, "add")
		done <- true
	}()
}

func (b *bess) delPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var any *anypb.Any
		var err error

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
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, "delete")
		done <- true
	}()
}

func (b *bess) processFAR(ctx context.Context, any *anypb.Any, method string) {
	if method != "add" && method != "delete" && method != "clear" {
		log.Println("Invalid method name: ", method)
		return
	}

	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "farLookup",
		Cmd:  method,
		Arg:  any,
	})
	if err != nil {
		log.Println("farLookup method failed!:", err)
	}
}

func (b *bess) addFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var any *anypb.Any
		var err error
		action := far.setActionValue()
		f := &pb.ExactMatchCommandAddArg{
			Gate: uint64(far.tunnelType),
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(uint64(far.fseID)), /* fseid */
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
		b.processFAR(ctx, any, "add")
		done <- true
	}()
}

func (b *bess) delFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.ExactMatchCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(uint64(far.fseID)), /* fseid */
			},
		}
		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		b.processFAR(ctx, any, "delete")
		done <- true
	}()
}

func (b *bess) processCounters(ctx context.Context, any *anypb.Any, method string, counterName string) {
	_, err := b.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: counterName,
		Cmd:  method,
		Arg:  any,
	})
	if err != nil {
		log.Println("counter method failed!:", err)
	}
}

func (b *bess) addCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.CounterAddArg{
			CtrId: ctrID,
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		b.processCounters(ctx, any, "add", counterName)
		done <- true
	}()
}

func (b *bess) delCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.CounterRemoveArg{
			CtrId: ctrID,
		}

		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		b.processCounters(ctx, any, "remove", counterName)
		done <- true
	}()
}

func (b *bess) removeAllPDRs(ctx context.Context, done chan<- bool) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.EmptyArg{}
		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processPDR(ctx, any, "clear")
		done <- true
	}()
}

func (b *bess) removeAllFARs(ctx context.Context, done chan<- bool) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.EmptyArg{}
		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processFAR(ctx, any, "clear")
		done <- true
	}()
}

func (b *bess) removeAllCounters(ctx context.Context, done chan<- bool, name string) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.EmptyArg{}
		any, err = anypb.New(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		b.processCounters(ctx, any, "removeAll", name)

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
