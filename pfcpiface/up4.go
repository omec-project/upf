// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"encoding/binary"
	"flag"
	"math/rand"
	"net"
	"time"

	p4 "github.com/p4lang/p4runtime/go/p4/v1"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

const (
	// FIXME: this is hardcoded currently, but should be passed as configuration/cmd line arg
	p4InfoPath       = "/bin/p4info.txt"
	deviceConfigPath = "/bin/bmv2.json"
)

var (
	p4RtcServerIP   = flag.String("p4RtcServerIP", "", "P4 Server ip")
	p4RtcServerPort = flag.String("p4RtcServerPort", "", "P4 Server port")
)

// FIXME: it should not be here IMO
// P4rtcInfo : P4 runtime interface settings.
type P4rtcInfo struct {
	AccessIP    string `json:"access_ip"`
	P4rtcServer string `json:"p4rtc_server"`
	P4rtcPort   string `json:"p4rtc_port"`
	UEIP        string `json:"ue_ip_pool"`
}

const (
	preQosCounterID = iota
	postQosCounterID

	// 253 base stations + 1 dbuf (fixed in UP4) + 1 reserved (fixed in P4 pipeline)
	maxGTPTunnelPeerIDs = 253
)

type counter struct {
	maxSize   uint64
	counterID uint64
	allocated map[uint64]uint64
	// free      map[uint64]uint64
}

type UP4 struct {
	host            string
	deviceID        uint64
	timeout         uint32
	accessIP        *net.IPNet
	p4rtcServer     string
	p4rtcPort       string
	enableEndMarker bool

	p4client       *P4rtClient
	p4RtTranslator *P4rtTranslator

	// TODO: create UP4Store object and move these fields there
	counters          []counter
	tunnelPeerIDs     map[tunnelParams]uint8
	tunnelPeerIDsPool []uint8

	// ueAddrToFSEID is used to store UE Address <-> F-SEID mapping,
	// which is needed to efficiently find F-SEID when we receive a P4 Digest (DDN) for a UE address.
	ueAddrToFSEID map[uint32]uint64

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

func (up4 *UP4) getAccessIP() (*net.IPNet, error) {
	log.Println("getAccessIP")

	interfaceTableEntry := up4.p4RtTranslator.BuildInterfaceTableEntryNoAction()

	resp, err := up4.p4client.ReadTableEntry(interfaceTableEntry)
	if err != nil {
		return nil, ErrOperationFailedWithReason("get Access IP from UP4", err.Error())
	}

	accessIP, err := up4.p4RtTranslator.ParseAccessIPFromReadInterfaceTableResponse(resp)
	if err != nil {
		return nil, err
	}

	return accessIP, nil
}

func (up4 *UP4) initCounter(counterID uint8, name string) error {
	ctr, err := up4.p4RtTranslator.getCounterByName(name)
	if err != nil {
		return err
	}

	up4.counters[counterID].maxSize = uint64(ctr.Size)
	up4.counters[counterID].counterID = uint64(ctr.Preamble.Id)

	log.WithFields(log.Fields{
		"counterID":      counterID,
		"name":           name,
		"max-size":       ctr.Size,
		"UP4 counter ID": ctr.Preamble.Id,
	}).Debug("Counter initialized successfully")

	return nil
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

func (up4 *UP4) setupChannel() error {
	log.Println("Channel Setup.")

	client, err := CreateChannel(up4.host, up4.deviceID)
	if err != nil {
		log.Errorf("create channel failed: %v", err)
		return err
	}

	up4.p4client = client

	err = up4.p4client.GetForwardingPipelineConfig()
	if err != nil {
		err = up4.p4client.SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath)
		if err != nil {
			log.Errorf("set forwarding pipeling config failed: %v", err)
			return err
		}
	}

	return nil
}

func (up4 *UP4) initAllCounters() error {
	log.Debug("Initializing counter for UP4")

	err := up4.initCounter(preQosCounterID, "PreQosPipe.pre_qos_counter")
	if err != nil {
		return ErrOperationFailedWithReason("init preQosCounterID counter", err.Error())
	}

	err = up4.initCounter(postQosCounterID, "PostQosPipe.post_qos_counter")
	if err != nil {
		return ErrOperationFailedWithReason("init postQosCounterID counter", err.Error())
	}

	return nil
}

func (up4 *UP4) initTunnelPeerIDs() {
	up4.tunnelPeerIDs = make(map[tunnelParams]uint8)
	// a simple queue storing available;
	// 0 is reserved;
	// 1 is reserved for dbuf
	up4.tunnelPeerIDsPool = make([]uint8, 0, maxGTPTunnelPeerIDs)

	for i := 2; i < maxGTPTunnelPeerIDs+2; i++ {
		up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, uint8(i))
	}
}

// This function ensures that PFCP Agent is connected to UP4.
// Returns true if the connection is already established.
// Otherwise, tries to connect to UP4. Returns false if fails.
// FIXME: the argument should be removed from fastpath API
func (up4 *UP4) isConnected(accessIP *net.IP) bool {
	if up4.p4client != nil {
		return true
	}

	err := up4.tryConnect()
	if err != nil {
		log.Errorf("failed to connect to UP4: %v", err)
		return false
	}

	return true
}

// TODO: rename it to initUPF()
func (up4 *UP4) setUpfInfo(u *upf, conf *Conf) {
	log.Println("setUpfInfo UP4")

	up4.accessIP = ParseStrIP(conf.P4rtcIface.AccessIP)
	u.accessIP = up4.accessIP.IP
	log.Println("AccessIP: ", up4.accessIP)

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
	up4.enableEndMarker = conf.EnableEndMarker
	up4.initTunnelPeerIDs()
	up4.ueAddrToFSEID = make(map[uint32]uint64)

	up4.counters = make([]counter, 2)
	for i := range up4.counters {
		// initialize allocated counters map
		up4.counters[i].allocated = make(map[uint64]uint64)
	}

	err := up4.tryConnect()
	if err != nil {
		log.Errorf("failed to connect to UP4: %v", err)
		return
	}

	if up4.accessIP != nil {
		u.accessIP = up4.accessIP.IP
	}
}

func (up4 *UP4) clearAllTables() error {
	sessionsUplinkTableID, err := up4.p4RtTranslator.getTableIDByName(TableUplinkSessions)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(sessionsUplinkTableID)
	if err != nil {
		return err
	}

	sessionsDownlinkTableID, err := up4.p4RtTranslator.getTableIDByName(TableDownlinkSessions)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(sessionsDownlinkTableID)
	if err != nil {
		return err
	}

	terminationsUplinkTableID, err := up4.p4RtTranslator.getTableIDByName(TableUplinkTerminations)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(terminationsUplinkTableID)
	if err != nil {
		return err
	}

	terminationsDownlinkTableID, err := up4.p4RtTranslator.getTableIDByName(TableDownlinkTerminations)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(terminationsDownlinkTableID)
	if err != nil {
		return err
	}

	gtpTunnelPeersTableID, err := up4.p4RtTranslator.getTableIDByName(TableTunnelPeers)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(gtpTunnelPeersTableID)
	if err != nil {
		return err
	}

	return nil
}

func (up4 *UP4) listenToDDNs() {
	log.Info("Listening to Data Notifications from UP4..")

	for {
		digestData := up4.p4client.GetNextDigestData()

		ueAddr := binary.BigEndian.Uint32(digestData)
		if fseid, exists := up4.ueAddrToFSEID[ueAddr]; exists {
			up4.reportNotifyChan <- fseid
		}
	}
}

func (up4 *UP4) tryConnect() error {
	err := up4.setupChannel()
	if err != nil {
		return err
	}

	up4.p4RtTranslator = newP4RtTranslator(up4.p4client.P4Info)

	err = up4.clearAllTables()
	if err != nil {
		log.Warningf("failed to clear tables: %v", err)
	}

	err = up4.initAllCounters()
	if err != nil {
		return ErrOperationFailedWithReason("counters initialization", err.Error())
	}

	go up4.listenToDDNs()

	if up4.enableEndMarker {
		log.Println("Starting end marker loop")

		up4.endMarkerChan = make(chan []byte, 1024)
		go up4.endMarkerSendLoop(up4.endMarkerChan)
	}

	up4.accessIP, err = up4.getAccessIP()
	if err != nil {
		log.Errorf("Failed to get Access IP from UP4: %v", err)
	} else {
		log.Infof("Retrieved Access IP from UP4: %v", up4.accessIP)
	}

	return nil
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

func findRelatedFAR(pdr pdr, fars []far) (far, error) {
	for _, far := range fars {
		if pdr.farID == far.farID {
			return far, nil
		}
	}

	return far{}, ErrNotFoundWithParam("related FAR for PDR", "PDR", pdr)
}

// Returns error if we reach maximum supported GTP Tunnel Peers.
func (up4 *UP4) allocateGTPTunnelPeerID() (uint8, error) {
	if len(up4.tunnelPeerIDsPool) == 0 {
		return 0, ErrOperationFailedWithReason("allocate GTP Tunnel Peer ID",
			"no free tunnel peer IDs available")
	}

	// pick top from queue
	allocated := up4.tunnelPeerIDsPool[0]
	up4.tunnelPeerIDsPool = up4.tunnelPeerIDsPool[1:]

	return allocated, nil
}

func (up4 *UP4) releaseAllocatedGTPTunnelPeerID(allocated uint8) {
	up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, allocated)
}

func (up4 *UP4) addOrUpdateGTPTunnelPeer(far far) error {
	var tunnelPeerID uint8

	methodType := p4.Update_MODIFY
	tunnelParams := tunnelParams{
		tunnelIP4Src: ip2int(up4.accessIP.IP),
		tunnelIP4Dst: far.tunnelIP4Dst,
		tunnelPort:   far.tunnelPort,
	}

	tunnelPeerID, exists := up4.tunnelPeerIDs[tunnelParams]

	if !exists {
		var err error

		tunnelPeerID, err = up4.allocateGTPTunnelPeerID()
		if err != nil {
			//log.Error("failed to allocate GTP tunnel peer ID based on FAR: ", err)
			return err
		}

		methodType = p4.Update_INSERT
	}

	gtpTunnelPeerEntry, err := up4.p4RtTranslator.BuildGTPTunnelPeerTableEntry(tunnelPeerID, tunnelParams)
	if err != nil {
		return err
	}

	if err := up4.p4client.ApplyTableEntries(methodType, gtpTunnelPeerEntry); err != nil {
		return err
	}

	up4.tunnelPeerIDs[tunnelParams] = tunnelPeerID

	return nil
}

func (up4 *UP4) removeGTPTunnelPeer(far far) {
	removeLog := log.WithFields(log.Fields{
		"far": far,
	})
	tunnelParams := tunnelParams{
		tunnelIP4Src: ip2int(up4.accessIP.IP),
		tunnelIP4Dst: far.tunnelIP4Dst,
		tunnelPort:   far.tunnelPort,
	}

	tunnelPeerID, exists := up4.tunnelPeerIDs[tunnelParams]
	if !exists {
		removeLog.WithField(
			"tunnel-params", tunnelParams).Error("GTP tunnel peer ID not found for tunnel params")
		return
	}

	removeLog.WithField("tunnel-peer-id", tunnelPeerID)

	gtpTunnelPeerEntry, err := up4.p4RtTranslator.BuildGTPTunnelPeerTableEntry(tunnelPeerID, tunnelParams)
	if err != nil {
		removeLog.Error("failed to build GTP tunnel peer entry to remove")
		return
	}

	removeLog.Debug("Removing GTP Tunnel Peer ID")

	if err := up4.p4client.ApplyTableEntries(p4.Update_DELETE, gtpTunnelPeerEntry); err != nil {
		removeLog.Error("failed to remove GTP tunnel peer")
	}

	up4.releaseAllocatedGTPTunnelPeerID(tunnelPeerID)
}

func (up4 *UP4) sendMsgToUPF(method upfMsgType, rules PacketForwardingRules, updated PacketForwardingRules) uint8 {
	up4Log := log.WithFields(log.Fields{
		"method-type":   method,
		"old-rules":     rules,
		"updated-rules": updated,
	})
	up4Log.Debug("Sending PFCP message to UP4..")

	pdrs := rules.pdrs
	fars := rules.fars

	if method == upfMsgTypeMod {
		// no need to use updated PDRs as session's PDRs are already updated
		fars = updated.fars
	}

	var (
		methodType p4.Update_Type
		err        error
		val        uint64
		cause      uint8 = ie.CauseRequestRejected
	)

	if !up4.isConnected(nil) {
		log.Error("UP4 server not connected")
		return cause
	}

	switch method {
	case upfMsgTypeAdd:
		{
			methodType = p4.Update_INSERT
			for i := range pdrs {
				val, err = getCounterVal(up4, preQosCounterID)
				if err != nil {
					log.Println("Counter id alloc failed ", err)
					return cause
				}
				pdrs[i].ctrID = uint32(val)
			}
		}
	case upfMsgTypeDel:
		{
			methodType = p4.Update_DELETE
			for i := range pdrs {
				resetCounterVal(up4, preQosCounterID,
					uint64(pdrs[i].ctrID))
			}
		}
	case upfMsgTypeMod:
		{
			methodType = p4.Update_MODIFY
		}
	default:
		{
			log.Println("Unknown method : ", method)
			return cause
		}
	}

	for _, far := range fars {
		// downlink FAR that does encapsulation
		if far.Forwards() && far.dstIntf == ie.DstInterfaceAccess {
			switch method {
			case upfMsgTypeAdd, upfMsgTypeMod:
				{
					if far.tunnelTEID == 0 {
						up4Log.Warn("Downlink FAR without tunnel params received, cannot install GTP Tunnel Peer")
						continue
					}
					if err := up4.addOrUpdateGTPTunnelPeer(far); err != nil {
						up4Log.WithFields(log.Fields{
							"far": far,
						}).Error("Failed to add or update GTP tunnel peer")
					}
				}
			case upfMsgTypeDel:
				{
					up4.removeGTPTunnelPeer(far)
				}
			default:
				up4Log.Errorf("unsupported PFCP method: %v", method)
			}
		}
	}

	for _, pdr := range pdrs {
		pdrLog := log.WithFields(log.Fields{
			"pdr": pdr,
		})
		pdrLog.Traceln(pdr)

		far, err := findRelatedFAR(pdr, fars)
		if err != nil {
			pdrLog.Warning("no related FAR for PDR found: ", err)
			continue
		}

		log.Println("Related FAR:")
		log.Println(far)

		tunnelParams := tunnelParams{
			tunnelIP4Src: ip2int(up4.accessIP.IP),
			tunnelIP4Dst: far.tunnelIP4Dst,
			tunnelPort:   far.tunnelPort,
		}

		tunnelPeerID, exists := up4.tunnelPeerIDs[tunnelParams]
		if !exists && far.Forwards() {
			pdrLog.Warn("related FAR does not include tunnel params, failed to find allocated GTP tunnel peer ID")
		}

		sessionsEntry, err := up4.p4RtTranslator.BuildSessionsTableEntry(pdr, tunnelPeerID, far.Buffers())
		if err != nil {
			log.Error("failed to build P4rt table entry for Sessions table: ", err)
			continue
		}

		// FIXME: get TC from QFI->TC mapping
		terminationsEntry, err := up4.p4RtTranslator.BuildTerminationsTableEntry(pdr, far, uint8(0))
		if err != nil {
			log.Error("failed to build P4rt table entry for Terminations table: ", err)
			continue
		}

		pdrLog.Traceln("Applying PDR: ", p4.Update_Type_name[int32(methodType)])

		err = up4.p4client.ApplyTableEntries(methodType, sessionsEntry, terminationsEntry)
		if err != nil {
			// TODO: revert operations (e.g. reset counter)
			log.Error("failed to write table entries to Sessions and Terminations tables: ", err)
			return cause
		}

		up4.saveUeAddrToFSEID(pdr)
	}

	cause = ie.CauseRequestAccepted

	return cause
}

func (up4 *UP4) saveUeAddrToFSEID(pdr pdr) {
	var ueAddr uint32
	if pdr.srcIface == access {
		ueAddr = pdr.srcIP
	} else if pdr.srcIface == core {
		ueAddr = pdr.dstIP
	} else {
		// unknown PDR direction
		return
	}

	if _, exists := up4.ueAddrToFSEID[ueAddr]; !exists {
		up4.ueAddrToFSEID[ueAddr] = pdr.fseID
	}
}
