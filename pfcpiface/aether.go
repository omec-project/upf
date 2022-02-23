// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math"
	"net"
)

type aether struct {
	bess
	accessIP net.IP
	ueSubnet *net.IPNet
}

const (
	// IP protocol types
	TcpProto = 6
	UdpProto = 17
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

func (a *aether) setUpfInfo(u *upf, conf *Conf) {
	a.bess.setUpfInfo(u, conf)
	var err error

	// GTP-U packets from eNB are uplink, to core.
	a.accessIP, _, err = net.ParseCIDR(conf.P4rtcIface.AccessIP)
	if err != nil {
		log.Fatalf("unable to parse IP %v that we should parse", conf.P4rtcIface.AccessIP)
	}

	// IP packets to UE subnet are downlink, from core.
	_, a.ueSubnet, err = net.ParseCIDR(u.ippoolCidr)
	if err != nil {
		log.Fatalln(err)
	}

	u.coreIP = net.ParseIP(net.IPv4zero.String())
	u.accessIP = a.accessIP

	u.enableFlowMeasure = true

	if err = a.setupInterfaceClassification(); err != nil {
		log.Fatalln(err)
	}
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
		ipProto:     UdpProto,
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
