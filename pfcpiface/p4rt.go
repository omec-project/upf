// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
)

//ctrType
const (
	preQosPdrCounter  uint8 = 0 //Pre qos pdr ctr
	postQosPdrCounter uint8 = 1 //Post qos pdr ctr
)

type counter struct {
	maxSize   uint64
	counterID uint64
	allocated map[uint64]uint64
	//free      map[uint64]uint64
}

type p4rtc struct {
	accessIP     net.IP
	accessIPMask net.IPMask
	n4SrcIP      net.IP
	coreIP       net.IP
	host         string
	fqdnh        string
	deviceID     uint64
	timeout      uint32
	msg_seq      uint32
	p4rtcServer  string
	p4rtcPort    string
	p4client     *P4rtClient
	counters     []counter
	upf          upf
	pfcpConn     *PFCPConn
	udpConn      *net.UDPConn
	udpAddr      net.Addr
}

func setSwitchInfo(p4rtClient *P4rtClient) (net.IP, net.IPMask, error) {
	log.Println("Set Switch Info")
	log.Println("device id ", (*p4rtClient).DeviceID)
	p4InfoPath := "/bin/p4info.txt"
	deviceConfigPath := "/bin/bmv2.json"

	errin := p4rtClient.GetForwardingPipelineConfig()
	if errin != nil {
		errin = p4rtClient.SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath)
		if errin != nil {
			log.Println("set forwarding pipeling config failed. ", errin)
			return nil, nil, errin
		}
	}

	intfEntry := IntfTableEntry{
		SrcIntf:   "ACCESS",
		Direction: "UPLINK",
	}

	errin = p4rtClient.ReadInterfaceTable(&intfEntry)
	if errin != nil {
		log.Println("Read Interface table failed ", errin)
		return nil, nil, errin
	}

	log.Println("accessip after read intf ", intfEntry.IP)
	accessIP := net.IP(intfEntry.IP)
	accessIPMask := net.CIDRMask(intfEntry.PrefixLen, 32)
	log.Println("AccessIP: ", accessIP, ", AccessIPMask: ", accessIPMask)

	return accessIP, accessIPMask, errin
}

func (c *counter) init() {
	c.allocated = make(map[uint64]uint64)
	//c.free = make(map[uint64]uint64)
}

func (p *p4rtc) setCounterSize(counterID uint8, name string) error {
	if p.p4client != nil {
		for _, ctr := range p.p4client.P4Info.Counters {
			if ctr.Preamble.Name == name {
				log.Println("maxsize : ", ctr.Size)
				log.Println("ctr ID : ", ctr.Preamble.Id)
				p.counters[counterID].maxSize = uint64(ctr.Size)
				p.counters[counterID].counterID = uint64(ctr.Preamble.Id)
				return nil
			}
		}
	}

	errin := fmt.Errorf("Countername not found %s", name)
	return errin
}

func (p *p4rtc) resetCounterVal(counterID uint8, val uint64) {
	log.Println("delete counter val ", val)
	//p.counters[counterID].allocated[val]=nil
	delete(p.counters[counterID].allocated, val)
	//p.counters[counterID].free[val] = 1
}

func (p *p4rtc) setInfo(conn *net.UDPConn, addr net.Addr, pconn *PFCPConn) {
	log.Println("setUDP Conn ", conn)
	p.udpConn = conn
	p.udpAddr = addr
	p.pfcpConn = pconn
}

func (p *p4rtc) getCounterVal(counterID uint8, pdrID uint32) (uint64, error) {
	/*
	   loop :
	      random counter generate
	      check allocated map
	      if not in map then return counter val.
	      if present continue
	      if loop reaches max break and fail.
	*/
	ctr := &p.counters[counterID]
	var val uint64
	for i := 0; i < int(ctr.maxSize); i++ {
		rand.Seed(time.Now().UnixNano())
		val = uint64(rand.Intn(int(ctr.maxSize)-1) + 1)
		if _, ok := ctr.allocated[val]; !ok {
			log.Println("key not in allocated map ", val)
			ctr.allocated[val] = 1
			return val, nil
		}
	}

	errin := fmt.Errorf("key alloc fail %v", val)
	return 0, errin
}

func (p *p4rtc) getAccessIP() net.IP {
	return p.accessIP
}

func (p *p4rtc) getAccessIPStr(val *string) {
	*val = p.accessIP.String()
}

func (p *p4rtc) exit() {
	log.Println("Exit function P4rtc")
}

func (p *p4rtc) getUpf() *upf {
	return &p.upf
}

func (p *p4rtc) getCoreIP(val *string) {
	*val = p.coreIP.String()
}

func (p *p4rtc) getN4SrcIP(val *string) {
	*val = p.n4SrcIP.String()
}

func (p *p4rtc) setUpfInfo(conf *Conf) {
	log.Println("setUpfInfo p4rtc")
	p.upf.accessIface = conf.AccessIface.IfName
	p.upf.coreIface = conf.CoreIface.IfName
	p.upf.accessIP = p.accessIP
	p.upf.coreIP = p.coreIP
	p.upf.fqdnHost = p.fqdnh
	p.upf.maxSessions = conf.MaxSessions
}

func channelSetup(p *p4rtc) (*P4rtClient, error) {
	log.Println("Channel Setup.")
	localclient, errin := CreateChannel(p.host,
		p.deviceID, p.timeout)
	if errin != nil {
		log.Println("create channel failed : ", errin)
		return nil, errin
	}
	if localclient != nil {
		log.Println("device id ", (*localclient).DeviceID)
		p.accessIP, p.accessIPMask, errin =
			setSwitchInfo(localclient)
		if errin != nil {
			log.Println("Switch set info failed ", errin)
			return nil, errin
		}
		log.Println("accessIP, Mask ", p.accessIP, p.accessIPMask)

	} else {
		log.Println("p4runtime client is null.")
		return nil, errin
	}

	return localclient, nil
}

func (p *p4rtc) initCounter() error {
	log.Println("Initialize counters for p4client.")
	var errin error
	if p.p4client == nil {
		errin = fmt.Errorf("Can't initialize counter. P4client null.")
		return errin
	}

	p.counters = make([]counter, 2)
	errin = p.setCounterSize(preQosPdrCounter,
		"PreQosPipe.pre_qos_pdr_counter")
	if errin != nil {
		log.Println("preQosPdrCounter counter not found : ", errin)
	}
	errin = p.setCounterSize(postQosPdrCounter,
		"PostQosPipe.post_qos_pdr_counter")
	if errin != nil {
		log.Println("postQosPdrCounter counter not found : ", errin)
	}
	for i := range p.counters {
		log.Println("init maps for counters.")
		p.counters[i].init()
	}

	return nil
}

func (p *p4rtc) handleChannelStatus() bool {
	var errin error
	if p.p4client == nil || p.p4client.CheckStatus() != Ready {
		p.p4client, errin = channelSetup(p)
		if errin != nil {
			log.Println("create channel failed : ", errin)
			return true
		}
		errin = p.initCounter()
		if errin != nil {
			log.Println("Counter Init failed. : ", errin)
			return true
		}
	}

	return false
}

func (p *p4rtc) sendDeleteAllSessionsMsgtoUPF() {
	log.Println("Loop through sessions and delete all entries p4")
	if (p.pfcpConn != nil) && (p.pfcpConn.mgr != nil) {
		for seidKey, value := range p.pfcpConn.mgr.sessions {
			p.sendMsgToUPF("del", value.pdrs, value.fars)
			p.pfcpConn.mgr.RemoveSession(seidKey)
		}
	}
}

func (p *p4rtc) parseFunc(conf *Conf) {
	log.Println("parseFunc p4rtc")
	var errin error
	p.accessIP, p.accessIPMask = ParseStrIP(conf.P4rtcIface.AccessIP)
	log.Println("AccessIP: ", p.accessIP,
		", AccessIPMask: ", p.accessIPMask)
	p.p4rtcServer = conf.P4rtcIface.P4rtcServer
	log.Println("p4rtc server ip/name", p.p4rtcServer)
	p.p4rtcPort = conf.P4rtcIface.P4rtcPort

	if *p4RtcServerIP != "" {
		p.p4rtcServer = *p4RtcServerIP
	}

	if *p4RtcServerPort != "" {
		p.p4rtcPort = *p4RtcServerPort
	}

	if *n4SrcIPStr != "" {
		p.n4SrcIP = net.ParseIP(*n4SrcIPStr)
	} else {
		p.n4SrcIP = net.ParseIP("0.0.0.0")
	}

	p.coreIP = net.ParseIP("0.0.0.0")
	log.Println("onos server ip ", p.p4rtcServer)
	log.Println("onos server port ", p.p4rtcPort)
	log.Println("n4 ip ", p.n4SrcIP.String())

	p.host = p.p4rtcServer + ":" + p.p4rtcPort
	log.Println("server name: ", p.host)
	p.deviceID = 1
	p.timeout = 30
	p.p4client, errin = channelSetup(p)
	if errin != nil {
		fmt.Printf("create channel failed : %v\n", errin)
	} else {
		errin = p.p4client.ClearPdrTable()
		if errin != nil {
			log.Println("clear PDR table failed : ", errin)
		}
		errin = p.p4client.ClearFarTable()
		if errin != nil {
			log.Println("clear FAR table failed : ", errin)
		}
	}

	errin = p.initCounter()
	if errin != nil {
		log.Println("Counter Init failed. : ", errin)
	}

	p.msg_seq = 0
}

func (p *p4rtc) sendMsgToUPF(method string, pdrs []pdr,
	fars []far) uint8 {
	log.Println("sendMsgToUPF p4")
	var funcType uint8
	var err error
	var val uint64
	var cause uint8 = ie.CauseRequestRejected
	var fseidIP uint32
	log.Println("Access IP ", p.accessIP.String())
	fseidIP = binary.LittleEndian.Uint32(p.accessIP.To4())
	log.Println("fseidIP ", fseidIP)
	switch method {
	case "add":
		{
			funcType = FunctionTypeInsert
			for i := range pdrs {
				//pdrs[i].fseidIP = fseidIP
				val, err = p.getCounterVal(
					preQosPdrCounter, pdrs[i].pdrID)
				if err != nil {
					log.Println("Counter id alloc failed ", err)
					return cause
				}
				pdrs[i].ctrID = uint32(val)
			}
		}
	case "del":
		{
			funcType = FunctionTypeDelete
			for i := range pdrs {
				p.resetCounterVal(preQosPdrCounter,
					uint64(pdrs[i].ctrID))
			}
		}
	case "mod":
		{
			funcType = FunctionTypeUpdate
		}
	default:
		{
			log.Println("Unknown method : ", method)
			return cause
		}
	}

	for _, pdr := range pdrs {
		errin := p.p4client.WritePdrTable(pdr, funcType)
		if errin != nil {
			log.Println("pdr entry function failed ", errin)
			return cause
		}
	}

	for _, far := range fars {
		far.printFAR()
		log.Println("write far funcType : ", funcType)
		errin := p.p4client.WriteFarTable(far, funcType)
		if errin != nil {
			log.Println("far entry function failed ", errin)
			return cause
		}
	}

	cause = ie.CauseRequestAccepted
	return cause
}
