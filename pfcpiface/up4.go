// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"flag"
	// #nosec G404 // Ignore G404. We don't need strong random number generator for allocating IDs for P4 objects.
	"math/rand"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

var (
	p4RtcServerIP   = flag.String("p4RtcServerIP", "", "P4 Server ip")
	p4RtcServerPort = flag.String("p4RtcServerPort", "", "P4 Server port")
)

// P4rtcInfo : P4 runtime interface settings.
type P4rtcInfo struct {
	AccessIP    string `json:"access_ip"`
	P4rtcServer string `json:"p4rtc_server"`
	P4rtcPort   string `json:"p4rtc_port"`
	UEIP        string `json:"ue_ip_pool"`
}

// TODO: convert uint8 to enum.
const (
	preQosPdrCounter  uint8 = 0 // Pre qos pdr ctr
	postQosPdrCounter uint8 = 1 // Post qos pdr ctr
)

type counter struct {
	maxSize   uint64
	counterID uint64
	allocated map[uint64]uint64
	// free      map[uint64]uint64
}

type UP4 struct {
	host             string
	deviceID         uint64
	timeout          uint32
	accessIPMask     net.IPMask
	accessIP         net.IP
	p4rtcServer      string
	p4rtcPort        string
	p4client         *P4rtClient
	counters         []counter
	reportNotifyChan chan<- uint64
	endMarkerChan    chan []byte
}

func (up4 *UP4) addSliceInfo(sliceInfo *SliceInfo) error {
	log.Errorln("Slice Info not supported in P4")
	return nil
}

func (up4 *UP4) summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric) {
}

func (up4 *UP4) sessionStats(uc *upfCollector, ch chan<- prometheus.Metric) error {
	return nil
}

func (up4 *UP4) portStats(uc *upfCollector, ch chan<- prometheus.Metric) {
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
}

func setCounterSize(p *UP4, counterID uint8, name string) error {
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

	errin := ErrNotFoundWithParam("counter", "name", name)

	return errin
}

func resetCounterVal(p *UP4, counterID uint8, val uint64) {
	log.Println("delete counter val ", val)
	delete(p.counters[counterID].allocated, val)
}

func getCounterVal(p *UP4, counterID uint8) (uint64, error) {
	/*
	   loop :
	      random counter generate
	      check allocated map
	      if not in map then return counter val.
	      if present continue
	      if loop reaches max break and fail.
	*/
	var val uint64

	ctr := &p.counters[counterID]
	for i := 0; i < int(ctr.maxSize); i++ {
		rand.Seed(time.Now().UnixNano())

		val = uint64(rand.Intn(int(ctr.maxSize)-1) + 1) // #nosec G404
		if _, ok := ctr.allocated[val]; !ok {
			log.Println("key not in allocated map ", val)

			ctr.allocated[val] = 1

			return val, nil
		}
	}

	return 0, ErrOperationFailedWithParam("counter allocation", "final val", val)
}

func (up4 *UP4) exit() {
	log.Println("Exit function P4rtc")
}

func (up4 *UP4) channelSetup() (*P4rtClient, error) {
	log.Println("Channel Setup.")

	localclient, errin := CreateChannel(up4.host, up4.deviceID, up4.reportNotifyChan)
	if errin != nil {
		log.Println("create channel failed : ", errin)
		return nil, errin
	}

	if localclient != nil {
		log.Println("device id ", (*localclient).DeviceID)

		up4.accessIP, up4.accessIPMask, errin = setSwitchInfo(localclient)
		if errin != nil {
			log.Println("Switch set info failed ", errin)
			return nil, errin
		}

		log.Println("accessIP, Mask ", up4.accessIP, up4.accessIPMask)
	} else {
		log.Println("p4runtime client is null.")
		return nil, errin
	}

	return localclient, nil
}

func initCounter(p *UP4) error {
	log.Println("Initialize counters for p4client.")

	var errin error

	if p.p4client == nil {
		return ErrOperationFailedWithReason("init counter", "P4client null")
	}

	p.counters = make([]counter, 2)

	errin = setCounterSize(p, preQosPdrCounter, "PreQosPipe.pre_qos_pdr_counter")
	if errin != nil {
		log.Println("preQosPdrCounter counter not found : ", errin)
	}

	errin = setCounterSize(p, postQosPdrCounter, "PostQosPipe.post_qos_pdr_counter")
	if errin != nil {
		log.Println("postQosPdrCounter counter not found : ", errin)
	}

	for i := range p.counters {
		log.Println("init maps for counters.")
		p.counters[i].init()
	}

	return nil
}

func (up4 *UP4) isConnected(accessIP *net.IP) bool {
	var errin error
	if up4.p4client == nil {
		up4.p4client, errin = up4.channelSetup()
		if errin != nil {
			log.Println("create channel failed : ", errin)
			return false
		}

		if accessIP != nil {
			*accessIP = up4.accessIP
		}

		errin = up4.p4client.ClearPdrTable()
		if errin != nil {
			log.Println("clear PDR table failed : ", errin)
		}

		errin = up4.p4client.ClearFarTable()
		if errin != nil {
			log.Println("clear FAR table failed : ", errin)
		}

		errin = initCounter(up4)
		if errin != nil {
			log.Println("Counter Init failed. : ", errin)
			return false
		}
	}

	return true
}

func (up4 *UP4) setUpfInfo(u *upf, conf *Conf) {
	log.Println("setUpfInfo UP4")

	var errin error

	u.accessIP, up4.accessIPMask = ParseStrIP(conf.P4rtcIface.AccessIP)
	log.Println("AccessIP: ", u.accessIP, ", AccessIPMask: ", up4.accessIPMask)

	up4.p4rtcServer = conf.P4rtcIface.P4rtcServer
	log.Println("UP4 server ip/name", up4.p4rtcServer)
	up4.p4rtcPort = conf.P4rtcIface.P4rtcPort
	up4.reportNotifyChan = u.reportNotifyChan

	if *p4RtcServerIP != "" {
		up4.p4rtcServer = *p4RtcServerIP
	}

	if *p4RtcServerPort != "" {
		up4.p4rtcPort = *p4RtcServerPort
	}

	u.coreIP = net.ParseIP(net.IPv4zero.String())

	log.Println("onos server ip ", up4.p4rtcServer)
	log.Println("onos server port ", up4.p4rtcPort)

	up4.host = up4.p4rtcServer + ":" + up4.p4rtcPort
	log.Println("server name: ", up4.host)
	up4.deviceID = 1
	up4.timeout = 30
	up4.p4client, errin = up4.channelSetup()
	u.accessIP = up4.accessIP

	if errin != nil {
		log.Println("create channel failed : ", errin)
	} else {
		errin = up4.p4client.ClearPdrTable()
		if errin != nil {
			log.Println("clear PDR table failed : ", errin)
		}

		errin = up4.p4client.ClearFarTable()
		if errin != nil {
			log.Println("clear FAR table failed : ", errin)
		}
	}

	errin = initCounter(up4)
	if errin != nil {
		log.Println("Counter Init failed. : ", errin)
	}

	if conf.EnableEndMarker {
		log.Println("Starting end marker loop")

		up4.endMarkerChan = make(chan []byte, 1024)
		go up4.endMarkerSendLoop(up4.endMarkerChan)
	}
}

func (up4 *UP4) sendEndMarkers(endMarkerList *[][]byte) error {
	for _, eMarker := range *endMarkerList {
		up4.endMarkerChan <- eMarker
	}

	return nil
}

func (up4 *UP4) endMarkerSendLoop(endMarkerChan chan []byte) {
	for outPacket := range endMarkerChan {
		err := up4.p4client.SendPacketOut(outPacket)
		if err != nil {
			log.Println("end marker write failed")
		}
	}
}

func (up4 *UP4) sendMsgToUPF(
	method upfMsgType, pdrs []pdr, fars []far, qers []qer) uint8 {
	log.Println("sendMsgToUPF p4")

	var (
		funcType uint8
		err      error
		val      uint64
		cause    uint8 = ie.CauseRequestRejected
	)

	if !up4.isConnected(nil) {
		log.Println("UP4 server not connected")
		return cause
	}

	switch method {
	case upfMsgTypeAdd:
		{
			funcType = FunctionTypeInsert
			for i := range pdrs {
				val, err = getCounterVal(up4, preQosPdrCounter)
				if err != nil {
					log.Println("Counter id alloc failed ", err)
					return cause
				}
				pdrs[i].ctrID = uint32(val)
			}
		}
	case upfMsgTypeDel:
		{
			funcType = FunctionTypeDelete
			for i := range pdrs {
				resetCounterVal(up4, preQosPdrCounter,
					uint64(pdrs[i].ctrID))
			}
		}
	case upfMsgTypeMod:
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
		log.Traceln(pdr)
		log.Traceln("write pdr funcType : ", funcType)

		errin := up4.p4client.WritePdrTable(pdr, funcType)
		if errin != nil {
			resetCounterVal(up4, preQosPdrCounter, uint64(pdr.ctrID))
			log.Println("pdr entry function failed ", errin)

			return cause
		}
	}

	for _, far := range fars {
		log.Traceln(far)
		log.Traceln("write far funcType : ", funcType)

		errin := up4.p4client.WriteFarTable(far, funcType)
		if errin != nil {
			log.Println("far entry function failed ", errin)
			return cause
		}
	}

	cause = ie.CauseRequestAccepted

	return cause
}
