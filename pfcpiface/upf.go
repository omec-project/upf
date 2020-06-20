// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"log"
	"strings"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

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
	ueAddrIP     uint32
	inetIP       uint32
	uePort       uint16
	inetPort     uint16
	proto        uint8

	srcIfaceMask     uint8
	tunnelIP4DstMask uint32
	eNBTeidMask      uint32
	ueAddrIPMask     uint32
	inetIPMask       uint32
	uePortMask       uint16
	inetPortMask     uint16
	protoMask        uint8

	pdrID     uint32
	fseID     uint32
	ctrID     uint32
	farID     uint32
	needDecap uint8
}

var _ pdr

var intEnc = func(u uint64) *pb.FieldData {
	return &pb.FieldData{Encoding: &pb.FieldData_ValueInt{ValueInt: u}}
}

func addPDR(ctx context.Context, c pb.BESSControlClient, sessionID uint32) {

	const ueip, teid = 0x10000001, 0xf0000000

	f := &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			intEnc(core),                     /* src_iface */
			{},                               /* tunnel_ipv4_dst */
			{},                               /* enb_teid */
			intEnc(uint64(ueip + sessionID)), /* ueaddr ip*/
			{},                               /* inet ip */
			{},                               /* ue port */
			{},                               /* inet port */
			{},                               /* proto id */
		},
		Masks: []*pb.FieldData{
			intEnc(0xFF),       /* src_iface-mask */
			{},                 /* tunnel_ipv4_dst-mask */
			{},                 /* enb_teid-mask */
			intEnc(0xFFFFFFFF), /* ueaddr ip-mask */
			{},                 /* inet ip-mask */
			{},                 /* ue port-mask */
			{},                 /* inet port-mask */
			{},                 /* proto id-mask */
		},
		Valuesv: []*pb.FieldData{
			{},                               /* pdr-id */
			intEnc(uint64(teid + sessionID)), /* fseid */
			intEnc(uint64(sessionID)),        /* ctr_id */
			intEnc(uint64(downlink)),         /* far_id */
			{},                               /* need_decap */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK PDR rule! Retrying...")
			return
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			intEnc(access),                   /* src_iface */
			{},                               /* tunnel_ipv4_dst */
			intEnc(uint64(teid + sessionID)), /* enb_teid */
			{},                               /* inet ip */
			intEnc(uint64(ueip + sessionID)), /* ueaddr ip */
			{},                               /* inet port */
			{},                               /* ue port */
			{},                               /* proto-id */
		},
		Masks: []*pb.FieldData{
			intEnc(0xFF),       /* src_iface-mask */
			{},                 /* tunnel_ipv4_dst-mask */
			intEnc(0xFFFFFFFF), /* enb_teid-mask */
			{},                 /* inet ip-mask */
			intEnc(0xFFFFFFFF), /* ueaddr ip-mask */
			{},                 /* inet port-mask */
			{},                 /* ue port-mask */
			{},                 /* proto-id-mask */
		},
		Valuesv: []*pb.FieldData{
			{},                               /* pdr_id */
			intEnc(uint64(teid + sessionID)), /* fseid */
			intEnc(uint64(sessionID)),        /* ctr_id */
			intEnc(uint64(uplink)),           /* far_id */
			intEnc(0x1),                      /* need_decap */
		},
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert UPLINK PDR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func addFAR(ctx context.Context, c pb.BESSControlClient, s1uip uint32, sessionID uint32) {

	const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80
	// NG4T-based formula to calculate enodeB IP address against a given UE IP address
	// il_trafficgen also uses the same scheme
	// See SimuCPEnbv4Teid(...) in ngic code for more details
	ueOfRan := sessionID % ng4tMaxUeRan
	ran := sessionID / ng4tMaxUeRan
	enbOfRan := ueOfRan % ng4tMaxEnbRan
	enbIdx := ran*ng4tMaxEnbRan + enbOfRan

	f := &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			intEnc(uint64(downlink)),         /* far_id */
			intEnc(uint64(teid + sessionID)), /* fseid */
		},
		Values: []*pb.FieldData{
			intEnc(farTunnel),                /* action */
			intEnc(0x1),                      /* tunnel_out_type */
			intEnc(uint64(s1uip)),            /* s1u-ip */
			intEnc(uint64(enbip + enbIdx)),   /* enb ip */
			intEnc(uint64(teid + sessionID)), /* enb teid */
			intEnc(uint64(udpGTPUPort)),      /* udp gtpu port */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK FAR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			intEnc(uint64(uplink)),           /* far id */
			intEnc(uint64(teid + sessionID)), /* fseid */
		},
		Values: []*pb.FieldData{
			intEnc(farForward), /* action */
			{},                 /* tunnel_out_type */
			{},                 /* not needed */
			{},                 /* not needed */
			{},                 /* not needed */
			{},                 /* not needed */
		},
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert UPLINK FAR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func addCounters(ctx context.Context, c pb.BESSControlClient, sessionID uint32) {

	f := &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PreQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PreQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostDLQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PostDLQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostULQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PostULQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}
