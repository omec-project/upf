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
}

func (b *bess) sendMsgToUPF(method string, pdrs []pdr, fars []far, u *upf) uint8 {
	// create context
	var cause uint8 = ie.CauseRequestAccepted
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	calls := len(pdrs) + len(fars)

	log.Println("upf : ", u.client)
	log.Println("conn : ", b.conn)
	// pause daemon, and then insert FAR(s), finally resume
	err := u.pauseAll()
	if err != nil {
		log.Fatalln("Unable to pause BESS:", err)
	}
	for _, pdr := range pdrs {
		switch method {
		case "add":
			fallthrough
		case "mod":
			u.addPDR(ctx, done, pdr)
		case "del":
			u.delPDR(ctx, done, pdr)
		}
	}
	for _, far := range fars {
		switch method {
		case "add":
			fallthrough
		case "mod":
			u.addFAR(ctx, done, far)
		case "del":
			u.delFAR(ctx, done, far)
		}
	}
	rc := u.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}
	err = u.resumeAll()
	if err != nil {
		log.Fatalln("Unable to resume BESS:", err)
	}

	return cause
}

func (b *bess) handleChannelStatus() bool {
	return false
}

func (b *bess) sendDeleteAllSessionsMsgtoUPF(upf *upf) {
	/* create context, pause daemon, insert PDR(s), and resume daemon */
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	done := make(chan bool)
	calls := 5

	err := upf.pauseAll()
	if err != nil {
		log.Fatalln("Unable to pause BESS:", err)
	}
	upf.removeAllPDRs(ctx, done)
	upf.removeAllFARs(ctx, done)
	upf.removeAllCounters(ctx, done, "preQoSCounter")
	upf.removeAllCounters(ctx, done, "postDLQoSCounter")
	upf.removeAllCounters(ctx, done, "postULQoSCounter")

	rc := upf.GRPCJoin(calls, Timeout, done)
	if !rc {
		log.Println("Unable to make GRPC calls")
	}
	err = upf.resumeAll()
	if err != nil {
		log.Fatalln("Unable to resume BESS:", err)
	}
}

func (b *bess) getAccessIP(val *string) {
	*val = b.accessIP.String()
}

func (b *bess) getCoreIP(val *string) {
	*val = b.coreIP.String()
}

func (b *bess) getN4SrcIP(val *string) {
	*val = b.n4SrcIP.String()
}

func (b *bess) getUpfInfo(conf *Conf, u *upf) {
	log.Println("getUpfInfo bess")
	u.accessIface = conf.AccessIface.IfName
	u.coreIface = conf.CoreIface.IfName
	u.accessIP = b.accessIP
	u.coreIP = b.coreIP
	u.fqdnHost = b.fqdnh
	u.client = pb.NewBESSControlClient(b.conn)
	u.maxSessions = conf.MaxSessions
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
	b.conn, errin = grpc.Dial(*bessIP, grpc.WithInsecure())
	if errin != nil {
		log.Fatalln("did not connect:", errin)
	}
	defer b.conn.Close()

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
}
