// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"log"
	"net"

	fqdn "github.com/Showmax/go-fqdn"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
)

type bess struct {
	accessIP net.IP
	n4SrcIP  net.IP
	coreIP   net.IP
	fqdnh    string
	conn     *grpc.ClientConn
	upfPt    *upf
}

func (b *bess) sendMsgToUPF(method string, pdrs []pdr, fars []far) uint8 {
	// create context
	var cause uint8 = ie.CauseRequestAccepted
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	calls := len(pdrs) + len(fars)

	log.Println("upf : ", b.upfPt.client)
	log.Println("conn : ", b.conn)
	// pause daemon, and then insert FAR(s), finally resume
	err := b.upfPt.pauseAll()
	if err != nil {
		log.Fatalln("Unable to pause BESS:", err)
	}
	for _, pdr := range pdrs {
		switch method {
		case "add":
			fallthrough
		case "mod":
			b.upfPt.addPDR(ctx, done, pdr)
		case "del":
			b.upfPt.delPDR(ctx, done, pdr)
		}
	}
	for _, far := range fars {
		switch method {
		case "add":
			fallthrough
		case "mod":
			b.upfPt.addFAR(ctx, done, far)
		case "del":
			b.upfPt.delFAR(ctx, done, far)
		}
	}
	rc := b.upfPt.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}
	err = b.upfPt.resumeAll()
	if err != nil {
		log.Fatalln("Unable to resume BESS:", err)
	}

	return cause
}

func (b *bess) handleChannelStatus() bool {
	return false
}

func (b *bess) sendDeleteAllSessionsMsgtoUPF() {
	/* create context, pause daemon, insert PDR(s), and resume daemon */
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	calls := 5

	err := b.upfPt.pauseAll()
	if err != nil {
		log.Fatalln("Unable to pause BESS:", err)
	}
	b.upfPt.removeAllPDRs(ctx, done)
	b.upfPt.removeAllFARs(ctx, done)
	b.upfPt.removeAllCounters(ctx, done, "preQoSCounter")
	b.upfPt.removeAllCounters(ctx, done, "postDLQoSCounter")
	b.upfPt.removeAllCounters(ctx, done, "postULQoSCounter")

	rc := b.upfPt.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}
	err = b.upfPt.resumeAll()
	if err != nil {
		log.Fatalln("Unable to resume BESS:", err)
	}
}

func (b *bess) getAccessIPStr(val *string) {
	*val = b.accessIP.String()
}

func (b *bess) getAccessIP() net.IP {
	return b.accessIP
}

func (b *bess) getUpf() *upf {
	return b.upfPt
}

func (b *bess) getCoreIP(val *string) {
	*val = b.coreIP.String()
}

func (b *bess) getN4SrcIP(val *string) {
	*val = b.n4SrcIP.String()
}

func (b *bess) setUpfInfo(conf *Conf) {
	log.Println("setUpfInfo bess")
	b.upfPt.accessIface = conf.AccessIface.IfName
	b.upfPt.coreIface = conf.CoreIface.IfName
	b.upfPt.accessIP = b.accessIP
	b.upfPt.coreIP = b.coreIP
	b.upfPt.fqdnHost = b.fqdnh
	b.upfPt.maxSessions = conf.MaxSessions

	var simInfo *SimModeInfo
	if conf.Mode == modeSim {
		simInfo = &conf.SimInfo
		b.upfPt.simInfo = simInfo
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		log.Println(*simulate, "sessions:", conf.MaxSessions)
		b.upfPt.sim(*simulate)
		return
	}
}

func (b *bess) exit() {
	log.Println("Exit function Bess")
	b.conn.Close()
}

func (b *bess) parseFunc(conf *Conf) {
	log.Println("parseFunc bess")
	b.accessIP = ParseIP(conf.AccessIface.IfName, "Access")
	b.coreIP = ParseIP(conf.CoreIface.IfName, "Core")

	// fetch fqdn. Prefer json field
	b.fqdnh = conf.CPIface.FQDNHost
	if b.fqdnh == "" {
		b.fqdnh = fqdn.Get()
	}

	// get bess grpc client
	var errin error
	b.upfPt = &upf{}
	log.Println("bessIP ", *bessIP)

	b.conn, errin = grpc.Dial(*bessIP, grpc.WithInsecure())
	if errin != nil {
		log.Fatalln("did not connect:", errin)
	}

	if conf.CPIface.SrcIP == "" {
		if conf.CPIface.DestIP != "" {
			b.n4SrcIP = getOutboundIP(conf.CPIface.DestIP)
		}
	} else {
		addrs, errin := net.LookupHost(conf.CPIface.SrcIP)
		if errin == nil {
			b.n4SrcIP = net.ParseIP(addrs[0])
		}
	}
	b.upfPt.client = pb.NewBESSControlClient(b.conn)
}
