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
	"github.com/wmnsk/go-pfcp/message"
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
	tickerT      *time.Ticker
	tickerDone   chan bool
	//reportURR    map[uint64]*reportRecord
	reportURRQ []*reportRecord
	reportChan chan *IntfCounterEntry
	udpConn    *net.UDPConn
	udpAddr    net.Addr
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

func (p *p4rtc) readCounterVal() {
	/* check for valid entries
		   readcounter only if any URRs registered is valid
	       deleting of entries will be done at dsr
	*/
	var flag bool = false

	for _, m := range p.reportURRQ {
		if !m.checkInvalid() {
			//delete(p.reportURR, key)
			flag = true
			break
		}
	}

	if flag {
		for idx, counter := range p.counters {
			var entry IntfCounterEntry
			entry.ByteCount = make([]uint64, p.counters[idx].maxSize)
			entry.PktCount = make([]uint64, p.counters[idx].maxSize)
			entry.CounterID = counter.counterID
			var err error
			var flag bool = false

			err = p.p4client.ReadCounter(&entry)
			if err == nil {
				flag = true
			}
			if err != nil {
				log.Println("Read counter failed for counter id : ", entry.CounterID)
			} else {
				if flag {
					log.Println("adding entry to report chan")
					PrintMemUsage()
					p.reportChan <- &entry
				}
			}
		}
	}
}

func (p *p4rtc) readReportChan() {
	for {
		select {
		case d := <-p.reportChan:
			//fmt.Println("got counter entry")
			p.handleCounterEntry(d)
			log.Println("handled counter entry from report chan")
			PrintMemUsage()

		default:
			time.Sleep(2 * time.Second)
			//log.Println("No reports to read")
		}
	}
}

/*
   delete session request.
   get URR report from map and delete
   add urr report to delete response.

*/
func (p *p4rtc) sendURRForReporting(recItem *reportRecord) {
	/*
	   add recItem to map.
	*/
	log.Println("sending URR for report : ", recItem.fseid)
	//fseidKey := recItem.fseid
	log.Println("adding urr report to map")
	PrintMemUsage()
	//p.reportURR[fseidKey] = recItem
	p.reportURRQ = append(p.reportURRQ, recItem)
}

func (p *p4rtc) setInfo(conn *net.UDPConn, addr net.Addr, pconn *PFCPConn) {
	log.Println("setUDP Conn ", conn)
	p.udpConn = conn
	p.udpAddr = addr
	p.pfcpConn = pconn
}

func (p *p4rtc) addUsageReports(
	sdRes *message.SessionDeletionResponse, seidKey uint64) {
	log.Println("Add Usage reports to sess del response")
	var element, rec *reportRecord
	//if _, ok := p.reportURR[seidKey]; !ok {
	if rec = p.getReportFromList(seidKey); rec == nil {
		log.Println("seidKey not matching : ", seidKey)
		return
	}
	var seqn uint32 = 0
	element = rec
	for _, urr := range *element.urrs {
		//log.Println("urrID : ", urr.urrID)
		//log.Println("urr.localVol : ", urr.localVol)
		sdRes.UsageReport = append(sdRes.UsageReport,
			ie.NewUsageReportWithinSessionReportRequest(
				ie.NewURRID(urr.urrID),
				ie.NewURSEQN(seqn),
				ie.NewVolumeMeasurement(totalVolume,
					urr.localVol,
					0, 0, 0, 0, 0),
			))
		seqn++
	}

	_ = p.removeReportFromList(seidKey)
}

func (p *p4rtc) handleVolQuotaExceed(pdrID uint32, keySeid uint64) {
	log.Println("handleVolQuotaExceed")
	sessItem := p.pfcpConn.mgr.sessions[keySeid]
	var found bool = false
	for _, pdr := range sessItem.pdrs {
		if pdr.pdrID == pdrID {
			for idx, farItem := range sessItem.fars {
				if farItem.farID == pdr.farID {
					log.Println("farID drop : ", farItem.farID)
					sessItem.fars[idx].applyAction = 0x01
					fars := make([]far, 0, 1)
					fars = append(fars, sessItem.fars[idx])
					p.sendMsgToUPF("add", nil, fars, nil)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
}

func (p *p4rtc) handleCounterEntry(ce *IntfCounterEntry) {
	log.Println("Handle counter Entry")
	/*
	   loop through map.
	   check if counter value >= volume thresh in urr.
	   if yes, send create report and send.
	   update curr volume to curr+thresh.
	   if no, do nothing.
	*/

	//var flag bool = false

	batchSize := 50
	//for _, element := range p.reportURR {
	var removeReport []*reportRecord
	var i int
	for i = 0; i < batchSize && i < len(p.reportURRQ); i++ {
		element := p.reportURRQ[i]
		removeReport = append(removeReport, element)
		log.Println("report URR ip , seid ",
			element.srcIP, element.fseid)
		var urrRec []urr
		var flag bool = false
		for idx, urr := range *element.urrs {
			log.Println("bytecount : ", ce.ByteCount[uint64(urr.ctrID)])
			log.Println("urr id : ", urr.urrID)
			log.Println("ctr id : ", urr.ctrID)
			log.Println("local thresh : ", urr.localThresh)
			(*element.urrs)[idx].localVol = ce.ByteCount[uint64(urr.ctrID)]
			if (urr.reportOpen && urr.measureM&measureVolume > 0) &&
				(urr.reportT.isVOLTHSet()) &&
				(urr.volThresh.flags&totalVolume > 0) &&
				(urr.localThresh <= ce.ByteCount[uint64(urr.ctrID)]) {
				log.Println("threshold reached.")
				flag = true
				urrRec = append(urrRec, urr)
				volVal := urr.localThresh + urr.volThresh.totalVol
				for volVal <= (*element.urrs)[idx].localVol {
					volVal = volVal + urr.volThresh.totalVol
				}
				if urr.reportT.isVOLQUSet() &&
					volVal >= urr.volQuota.totalVol {
					log.Println("more than quota")
					(*element.urrs)[idx].reportOpen = false
					p.handleVolQuotaExceed(urr.pdrID, element.fseid)
				} else if urr.reportT.isVOLQUSet() {
					(*element.urrs)[idx].localThresh = volVal
					//log.Println("less than quota next thresh ", volVal)
				} else {
					(*element.urrs)[idx].localThresh = urr.volThresh.totalVol
				}
			}
		}
		if flag {
			p.sendSessRepReq(urrRec, element.fseid)
		}
	}

	if i < len(p.reportURRQ) {
		p.reportURRQ = p.reportURRQ[i:len(p.reportURRQ)]
	} else {
		p.reportURRQ = nil
	}

	p.reportURRQ = append(p.reportURRQ, removeReport...)
	ce.ByteCount = nil
	ce.PktCount = nil
}

func (p *p4rtc) sendSessRepReq(urrRec []urr, fseid uint64) {
	log.Println("Send Sess rep for fseid : ", fseid)
	serep := message.NewSessionReportRequest(0, /* MO?? <-- what's this */
		0,                            /* FO <-- what's this? */
		0,                            /* seid */
		p.msg_seq,                    /* seq # */
		0,                            /* priority */
		ie.NewReportType(0, 0, 1, 0), /*upir, erir, usar, dldr int*/
	)
	serep.Header.SEID = fseid
	var seqn uint32 = 0
	for _, urr := range urrRec {
		serep.UsageReport = append(serep.UsageReport, ie.NewUsageReportWithinSessionReportRequest(
			ie.NewURRID(urr.urrID),
			ie.NewURSEQN(seqn),
			ie.NewVolumeMeasurement(totalVolume, urr.localVol,
				0, 0, 0, 0, 0),
		))
		seqn++
	}
	ret, err := serep.Marshal()
	if err != nil {
		log.Println("Marshal function failed for SM resp ", err)
	}

	// send the report req out
	if ret != nil {
		if _, err := p.udpConn.WriteTo(ret, p.udpAddr); err != nil {
			log.Fatalln("Unable to transmit Report req", err)
		}
		p.msg_seq++
	}
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
	close(p.tickerDone)
	p.tickerT.Stop()
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

func (p *p4rtc) getReportFromList(seidKey uint64) *reportRecord {
	log.Println("get report for seid : ", seidKey)
	for _, val := range p.reportURRQ {
		if val.fseid == seidKey {
			return val
		}
	}

	return nil
}

func (p *p4rtc) removeReportFromList(seidKey uint64) *reportRecord {
	log.Println("Remove report from list : ", seidKey)
	index := -1
	var rec *reportRecord
	for idx, val := range p.reportURRQ {
		if val.fseid == seidKey {
			index = idx
			rec = val
			break
		}
	}

	if index == -1 {
		return nil
	}
	if len(p.reportURRQ) == 1 {
		p.reportURRQ = nil
	} else {
		p.reportURRQ = append(p.reportURRQ[:index],
			p.reportURRQ[index+1:]...)
	}

	return rec
}

func (p *p4rtc) sendDeleteAllSessionsMsgtoUPF() {
	log.Println("Loop through sessions and delete all entries p4")
	if (p.pfcpConn != nil) && (p.pfcpConn.mgr != nil) {
		for seidKey, value := range p.pfcpConn.mgr.sessions {
			p.sendMsgToUPF("del", value.pdrs, value.fars, value.urrs)
			_ = p.removeReportFromList(seidKey)
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

	p.reportChan = make(chan *IntfCounterEntry, 10)
	p.msg_seq = 0
	p.tickerDone = make(chan bool)
	p.tickerT = schedule(p.readCounterVal, 10*time.Second, p.tickerDone)
	go p.readReportChan()
}

func (p *p4rtc) sendMsgToUPF(method string, pdrs []pdr,
	fars []far, urrs []urr) uint8 {
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
				for m := range urrs {
					if urrs[m].urrID == uint32(pdrs[i].urrID) {
						urrs[m].pdrID = pdrs[i].pdrID
						urrs[m].ctrID = pdrs[i].ctrID
						break
					}
				}
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
