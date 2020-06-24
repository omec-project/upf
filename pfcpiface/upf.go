// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"encoding/binary"
	"log"
	"net"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

type upf struct {
	n3IP        net.IP
	client      pb.BESSControlClient
	ctx         context.Context
	maxSessions uint32
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

func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func (u *upf) pauseAll() error {
	_, err := u.client.PauseAll(u.ctx, &pb.EmptyRequest{})
	return err
}

func (u *upf) resumeAll() error {
	_, err := u.client.ResumeAll(u.ctx, &pb.EmptyRequest{})
	return err
}

func (u *upf) processPDR(any *anypb.Any, method string) {

	if method != "add" && method != "delete" {
		log.Println("Invalid method name: ", method)
		return
	}

	for {

		res, err := u.client.ModuleCommand(u.ctx, &pb.CommandRequest{
			Name: "PDRLookup",
			Cmd:  method,
			Arg:  any,
		})

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK PDR rule! Retrying...")
		}

		if res.GetError() != nil {
			if res.GetError().Code == 2 {
				log.Println(res.GetError(), "Retrying...")
				time.Sleep(1 * time.Second)
				continue
			}
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func (u *upf) addPDR(p pdr) {
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

	u.processPDR(any, "add")
}

func (u *upf) delPDR(p pdr) {
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

	u.processPDR(any, "delete")
}

func (u *upf) processFAR(any *anypb.Any, method string) {

	if method != "add" && method != "delete" {
		log.Println("Invalid method name: ", method)
		return
	}

	for {
		res, err := u.client.ModuleCommand(u.ctx, &pb.CommandRequest{
			Name: "FARLookup",
			Cmd:  method,
			Arg:  any,
		})

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK FAR rule! Retrying...")
		}

		if res.GetError() != nil {
			if res.GetError().Code == 2 {
				log.Println(res.GetError(), "Retrying...")
				time.Sleep(1 * time.Second)
				continue
			}
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func (u *upf) addFAR(far far) {
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
	u.processFAR(any, "add")
}

func (u *upf) delFAR(far far) {
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
	u.processFAR(any, "delete")
}

func (u *upf) processCounters(any *anypb.Any, method string, counterName string) {

	for {
		res, err := u.client.ModuleCommand(u.ctx, &pb.CommandRequest{
			Name: counterName,
			Cmd:  method,
			Arg:  any,
		})

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PreQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			if res.GetError().Code == 2 {
				log.Println(res.GetError(), "Retrying...")
				time.Sleep(1 * time.Second)
				continue
			}
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func (u *upf) addCounter(ctrID uint32, counterName string) {
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
	u.processCounters(any, "add", counterName)
}

func (u *upf) delCounter(ctrID uint32, counterName string) {
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
	u.processCounters(any, "remove", counterName)
}
