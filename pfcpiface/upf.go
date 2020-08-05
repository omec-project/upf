// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/protobuf/types/known/anypb"
)

type upf struct {
	n3Iface     string
	n6Iface     string
	n3IP        net.IP
	client      pb.BESSControlClient
	maxSessions uint32
	simInfo     *SimModeInfo
}

// to be replaced with go-pfcp structs

// Don't change these values
const (
	udpGTPUPort = 2152

	// src-iface consts
	core   = 0x2
	access = 0x1

	// far-id specific directions
	uplink   = 0x0
	downlink = 0x1

	// far-action specific values
	farForward = 0x0
	farTunnel  = 0x1
)

type pdr struct {
	srcIface     uint8
	tunnelIP4Dst uint32
	eNBTeid      uint32
	srcIP        uint32
	dstIP        uint32
	fseidIP      uint32
	srcPort      uint16
	dstPort      uint16
	proto        uint8

	srcIfaceMask     uint8
	tunnelIP4DstMask uint32
	eNBTeidMask      uint32
	srcIPMask        uint32
	dstIPMask        uint32
	srcPortMask      uint16
	dstPortMask      uint16
	protoMask        uint8

	pdrID     uint32
	fseID     uint32
	ctrID     uint32
	farID     uint8
	needDecap uint8
}

type far struct {
	farID uint8
	fseID uint32

	action      uint8
	tunnelType  uint8
	s1uIP       uint32
	eNBIP       uint32
	eNBTeid     uint32
	UDPGTPUPort uint16
}

var intEnc = func(u uint64) *pb.FieldData {
	return &pb.FieldData{Encoding: &pb.FieldData_ValueInt{ValueInt: u}}
}

func (u *upf) sim(method string) {
	start := time.Now()

	// Pause workers before
	u.pauseAll()

	//const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	ueip, teid, enbip := net.ParseIP(u.simInfo.StartUeIP), hex2int(u.simInfo.StartTeid), net.ParseIP(u.simInfo.StartEnodeIP)
	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80
	s1uip := ip2int(u.n3IP)

	for i := uint32(0); i < u.maxSessions; i++ {
		// NG4T-based formula to calculate enodeB IP address against a given UE IP address
		// il_trafficgen also uses the same scheme
		// See SimuCPEnbv4Teid(...) in ngic code for more details
		ueOfRan := i % ng4tMaxUeRan
		ran := i / ng4tMaxUeRan
		enbOfRan := ueOfRan % ng4tMaxEnbRan
		enbIdx := ran*ng4tMaxEnbRan + enbOfRan

		// create/delete downlink pdr
		pdrDown := pdr{
			srcIface:     core,
			srcIP:        ip2int(ueip) + i,
			srcIfaceMask: 0xFF,
			srcIPMask:    0xFFFFFFFF,
			fseID:        teid + i,
			ctrID:        i,
			farID:        downlink,
		}

		// create/delete uplink pdr
		pdrUp := pdr{
			srcIface:     access,
			eNBTeid:      teid + i,
			dstIP:        ip2int(ueip) + i,
			srcIfaceMask: 0xFF,
			eNBTeidMask:  0xFFFFFFFF,
			dstIPMask:    0xFFFFFFFF,
			fseID:        teid + i,
			ctrID:        i,
			farID:        uplink,
		}

		// create/delete downlink far
		farDown := far{
			farID:       downlink,
			fseID:       teid + i,
			action:      farTunnel,
			tunnelType:  0x1,
			s1uIP:       s1uip,
			eNBIP:       ip2int(enbip) + enbIdx,
			eNBTeid:     teid + i,
			UDPGTPUPort: udpGTPUPort,
		}

		// create/delete uplink far
		farUp := far{
			farID:  uplink,
			fseID:  teid + i,
			action: farForward,
		}

		switch timeout := 100 * time.Millisecond; method {
		case "create":
			u.createEntries(pdrDown, pdrUp, farDown, farUp, timeout)

		case "delete":
			u.deleteEntries(pdrDown, pdrUp, farDown, farUp, timeout)

		default:
			log.Fatalln("Unsupported method", method)
		}
	}
	u.resumeAll()

	log.Println("Sessions/s:", float64(u.maxSessions)/time.Since(start).Seconds())
}

func (u *upf) createEntries(pdrDown, pdrUp pdr, farDown, farUp far, timeout time.Duration) {
	calls := 7
	boom := time.After(timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool)

	u.addPDR(ctx, done, pdrDown)
	u.addPDR(ctx, done, pdrUp)

	u.addFAR(ctx, done, farDown)
	u.addFAR(ctx, done, farUp)

	u.addCounter(ctx, done, pdrDown.ctrID, "preQoSCounter")
	u.addCounter(ctx, done, pdrDown.ctrID, "postDLQoSCounter")
	u.addCounter(ctx, done, pdrDown.ctrID, "postULQoSCounter")

	for {
		select {
		case ok := <-done:
			if !ok {
				log.Println("Error adding entries")
				cancel()
				go u.deleteEntries(pdrDown, pdrUp, farDown, farUp, timeout)
				return
			}
			calls = calls - 1
			if calls == 0 {
				return
			}
		case <-boom:
			log.Println("Timed out adding entries")
			return
		}
	}
}

func (u *upf) deleteEntries(pdrDown, pdrUp pdr, farDown, farUp far, timeout time.Duration) {
	calls := 7
	boom := time.After(timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool)

	u.delPDR(ctx, done, pdrDown)
	u.delPDR(ctx, done, pdrUp)

	u.delFAR(ctx, done, farDown)
	u.delFAR(ctx, done, farUp)

	u.delCounter(ctx, done, pdrDown.ctrID, "preQoSCounter")
	u.delCounter(ctx, done, pdrDown.ctrID, "postDLQoSCounter")
	u.delCounter(ctx, done, pdrDown.ctrID, "postULQoSCounter")

	for {
		select {
		case ok := <-done:
			if !ok {
				log.Println("Unable to delete entries")
			}
			calls = calls - 1
			if calls == 0 {
				return
			}
		case <-boom:
			return
		}
	}
}

func (u *upf) pauseAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := u.client.PauseAll(ctx, &pb.EmptyRequest{})
	return err
}

func (u *upf) resumeAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := u.client.ResumeAll(ctx, &pb.EmptyRequest{})
	return err
}

func (u *upf) processPDR(ctx context.Context, any *anypb.Any, method string) {

	if method != "add" && method != "delete" {
		log.Println("Invalid method name: ", method)
		return
	}

	u.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "pdrLookup",
		Cmd:  method,
		Arg:  any,
	})
}

func (u *upf) addP4PDR(ctx context.Context, done chan<- bool, p pdr) error {
    log.Println("Add P4 PDR entry")
    return client.WritePdrTable(p, FUNCTION_TYPE_INSERT)
}

func (u *upf) addPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.WildcardMatchCommandAddArg{
			Gate:     1,
			Priority: 1,
			Values: []*pb.FieldData{
				intEnc(uint64(p.srcIface)),     /* src_iface */
				intEnc(uint64(p.tunnelIP4Dst)), /* tunnel_ipv4_dst */
				intEnc(uint64(p.eNBTeid)),      /* enb_teid */
				intEnc(uint64(p.srcIP)),        /* ueaddr ip*/
				intEnc(uint64(p.dstIP)),        /* inet ip */
				intEnc(uint64(p.srcPort)),      /* ue port */
				intEnc(uint64(p.dstPort)),      /* inet port */
				intEnc(uint64(p.proto)),        /* proto id */
			},
			Masks: []*pb.FieldData{
				intEnc(uint64(p.srcIfaceMask)),     /* src_iface-mask */
				intEnc(uint64(p.tunnelIP4DstMask)), /* tunnel_ipv4_dst-mask */
				intEnc(uint64(p.eNBTeidMask)),      /* enb_teid-mask */
				intEnc(uint64(p.srcIPMask)),        /* ueaddr ip-mask */
				intEnc(uint64(p.dstIPMask)),        /* inet ip-mask */
				intEnc(uint64(p.srcPortMask)),      /* ue port-mask */
				intEnc(uint64(p.dstPortMask)),      /* inet port-mask */
				intEnc(uint64(p.protoMask)),        /* proto id-mask */
			},
			Valuesv: []*pb.FieldData{
				intEnc(uint64(p.pdrID)),     /* pdr-id */
				intEnc(uint64(p.fseID)),     /* fseid */
				intEnc(uint64(p.ctrID)),     /* ctr_id */
				intEnc(uint64(p.farID)),     /* far_id */
				intEnc(uint64(p.needDecap)), /* need_decap */
			},
		}
		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		u.processPDR(ctx, any, "add")
		done <- true
	}()
}

func (u *upf) delPDR(ctx context.Context, done chan<- bool, p pdr) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.WildcardMatchCommandDeleteArg{
			Values: []*pb.FieldData{
				intEnc(uint64(p.srcIface)),     /* src_iface */
				intEnc(uint64(p.tunnelIP4Dst)), /* tunnel_ipv4_dst */
				intEnc(uint64(p.eNBTeid)),      /* enb_teid */
				intEnc(uint64(p.srcIP)),        /* ueaddr ip*/
				intEnc(uint64(p.dstIP)),        /* inet ip */
				intEnc(uint64(p.srcPort)),      /* ue port */
				intEnc(uint64(p.dstPort)),      /* inet port */
				intEnc(uint64(p.proto)),        /* proto id */
			},
			Masks: []*pb.FieldData{
				intEnc(uint64(p.srcIfaceMask)),     /* src_iface-mask */
				intEnc(uint64(p.tunnelIP4DstMask)), /* tunnel_ipv4_dst-mask */
				intEnc(uint64(p.eNBTeidMask)),      /* enb_teid-mask */
				intEnc(uint64(p.srcIPMask)),        /* ueaddr ip-mask */
				intEnc(uint64(p.dstIPMask)),        /* inet ip-mask */
				intEnc(uint64(p.srcPortMask)),      /* ue port-mask */
				intEnc(uint64(p.dstPortMask)),      /* inet port-mask */
				intEnc(uint64(p.protoMask)),        /* proto id-mask */
			},
		}
		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}

		u.processPDR(ctx, any, "delete")
		done <- true
	}()
}

func (u *upf) processFAR(ctx context.Context, any *anypb.Any, method string) {

	if method != "add" && method != "delete" {
		log.Println("Invalid method name: ", method)
		return
	}

	u.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "farLookup",
		Cmd:  method,
		Arg:  any,
	})
}

func (u *upf) addFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.ExactMatchCommandAddArg{
			Gate: 1,
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(uint64(far.fseID)), /* fseid */
			},
			Values: []*pb.FieldData{
				intEnc(uint64(far.action)),      /* action */
				intEnc(uint64(far.tunnelType)),  /* tunnel_out_type */
				intEnc(uint64(far.s1uIP)),       /* s1u-ip */
				intEnc(uint64(far.eNBIP)),       /* enb ip */
				intEnc(uint64(far.eNBTeid)),     /* enb teid */
				intEnc(uint64(far.UDPGTPUPort)), /* udp gtpu port */
			},
		}
		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		u.processFAR(ctx, any, "add")
		done <- true
	}()
}

func (u *upf) delFAR(ctx context.Context, done chan<- bool, far far) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.ExactMatchCommandDeleteArg{
			Fields: []*pb.FieldData{
				intEnc(uint64(far.farID)), /* far_id */
				intEnc(uint64(far.fseID)), /* fseid */
			},
		}
		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		u.processFAR(ctx, any, "delete")
		done <- true
	}()
}

func (u *upf) processCounters(ctx context.Context, any *anypb.Any, method string, counterName string) {
	u.client.ModuleCommand(ctx, &pb.CommandRequest{
		Name: counterName,
		Cmd:  method,
		Arg:  any,
	})
}

func (u *upf) addCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.CounterAddArg{
			CtrId: ctrID,
		}

		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		u.processCounters(ctx, any, "add", counterName)
		done <- true
	}()
}

func (u *upf) delCounter(ctx context.Context, done chan<- bool, ctrID uint32, counterName string) {
	go func() {
		var any *anypb.Any
		var err error

		f := &pb.CounterRemoveArg{
			CtrId: ctrID,
		}

		any, err = ptypes.MarshalAny(f)
		if err != nil {
			log.Println("Error marshalling the rule", f, err)
			return
		}
		u.processCounters(ctx, any, "remove", counterName)
		done <- true
	}()
}

func (u *upf) measure(ifName string, f *pb.MeasureCommandGetSummaryArg) *pb.MeasureCommandGetSummaryResponse {
	modName := func() string {
		return ifName + "_measure"
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return nil
	}

	ctx := context.Background()
	modRes, err := u.client.ModuleCommand(ctx, &pb.CommandRequest{
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

func (u *upf) portStats(ifname string) *pb.GetPortStatsResponse {
	req := &pb.GetPortStatsRequest{
		Name: ifname + "Fast",
	}
	ctx := context.Background()
	res, err := u.client.GetPortStats(ctx, req)
	if err != nil || res.GetError() != nil {
		log.Println("Error calling GetPortStats", ifname, err, res.GetError().Errmsg)
		return nil
	}
	return res
}
