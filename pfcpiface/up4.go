// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"encoding/binary"
	"flag"
	"math"
	"math/rand"
	"net"
	"time"

	"google.golang.org/grpc/codes"

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

const (
	preQosCounterID = iota
	postQosCounterID

	// 253 base stations + 1 dbuf (fixed in UP4) + 1 reserved (fixed in P4 pipeline)
	maxGTPTunnelPeerIDs = 253
	maxApplicationIDs   = 254
)

type application struct {
	appIP     uint32
	appL4Port portRange
	appProto  uint8
}

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
	ueIPPool        *net.IPNet
	p4rtcServer     string
	p4rtcPort       string
	enableEndMarker bool

	p4client       *P4rtClient
	p4RtTranslator *P4rtTranslator

	// TODO: create UP4Store object and move these fields there
	counters           []counter
	tunnelPeerIDs      map[tunnelParams]uint8
	tunnelPeerIDsPool  []uint8
	applicationIDs     map[application]uint8
	applicationIDsPool []uint8

	// ueAddrToFSEID is used to store UE Address <-> F-SEID mapping,
	// which is needed to efficiently find F-SEID when we receive a P4 Digest (DDN) for a UE address.
	ueAddrToFSEID map[uint32]uint64
	// fseidToUEAddr is used to store F-SEID <-> UE Address mapping,
	// which is needed to efficiently find UE address for UL PDRs in the PFCP messages.
	// We need both maps to make lookup efficient, but both maps should always be updated in atomic way.
	fseidToUEAddr map[uint64]uint32

	reportNotifyChan chan<- uint64
	endMarkerChan    chan []byte
}

func (up4 *UP4) addSliceInfo(sliceInfo *SliceInfo) error {
	log.Errorln("Slice Info not supported in P4")
	return nil
}

func (up4 *UP4) summaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric) {
}

func (up4 *UP4) sessionStats(*PfcpNodeCollector, chan<- prometheus.Metric) error {
	return nil
}

func (up4 *UP4) portStats(uc *upfCollector, ch chan<- prometheus.Metric) {
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
	// a simple queue storing available tunnel peer IDs
	// 0 is reserved;
	// 1 is reserved for dbuf
	up4.tunnelPeerIDsPool = make([]uint8, 0, maxGTPTunnelPeerIDs)

	for i := 2; i < maxGTPTunnelPeerIDs+2; i++ {
		up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, uint8(i))
	}
}

func (up4 *UP4) initApplicationIDs() {
	up4.applicationIDs = make(map[application]uint8)
	// a simple queue storing available application IDs
	// 0 is reserved;
	up4.applicationIDsPool = make([]uint8, 0, maxApplicationIDs)

	for i := 1; i < maxApplicationIDs+1; i++ {
		up4.applicationIDsPool = append(up4.applicationIDsPool, uint8(i))
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

	up4.accessIP = MustParseStrIP(conf.P4rtcIface.AccessIP)
	u.accessIP = up4.accessIP.IP

	log.Println("AccessIP: ", up4.accessIP)

	up4.ueIPPool = MustParseStrIP(conf.CPIface.UEIPPool)

	log.Infof("UE IP pool: %v", up4.ueIPPool)

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
	up4.initApplicationIDs()
	up4.ueAddrToFSEID = make(map[uint32]uint64)
	up4.fseidToUEAddr = make(map[uint64]uint32)

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

	applicationsTableID, err := up4.p4RtTranslator.getTableIDByName(TableApplications)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(applicationsTableID)
	if err != nil {
		return err
	}

	interfacesTableID, err := up4.p4RtTranslator.getTableIDByName(TableInterfaces)
	if err != nil {
		return err
	}

	err = up4.p4client.ClearTable(interfacesTableID)
	if err != nil {
		return err
	}

	return nil
}

func (up4 *UP4) initUEPool() error {
	entry, err := up4.p4RtTranslator.BuildInterfaceTableEntry(up4.ueIPPool, true)
	if err != nil {
		return err
	}

	if err := up4.p4client.ApplyTableEntries(p4.Update_INSERT, entry); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"ue pool": up4.ueIPPool,
	}).Debug("UE pool successfully initialized in the UP4 pipeline")

	return nil
}

func (up4 *UP4) initN3Address() error {
	entry, err := up4.p4RtTranslator.BuildInterfaceTableEntry(up4.accessIP, false)
	if err != nil {
		return err
	}

	if err := up4.p4client.ApplyTableEntries(p4.Update_INSERT, entry); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"N3 address": up4.accessIP,
	}).Debug("N3 address successfully initialized in the UP4 pipeline")

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

	err = up4.initUEPool()
	if err != nil {
		return ErrOperationFailedWithReason("UE pool initialization", err.Error())
	}

	err = up4.initN3Address()
	if err != nil {
		return ErrOperationFailedWithReason("N3 address initialization", err.Error())
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

// FIXME: SDFAB-960
//nolint:unused
func (up4 *UP4) releaseAllocatedGTPTunnelPeerID(tunnelParams tunnelParams) {
	allocated, exists := up4.tunnelPeerIDs[tunnelParams]
	if exists {
		delete(up4.tunnelPeerIDs, tunnelParams)
		up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, allocated)
	}
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

// FIXME: SDFAB-960
//nolint:unused
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

	up4.releaseAllocatedGTPTunnelPeerID(tunnelParams)
}

// Returns error if we reach maximum supported Application IDs.
func (up4 *UP4) allocateInternalApplicationID(app application) (uint8, error) {
	if len(up4.applicationIDsPool) == 0 {
		return 0, ErrOperationFailedWithReason("allocate Application ID",
			"no free application IDs available")
	}

	// pick top from queue
	allocated := up4.applicationIDsPool[0]
	up4.applicationIDsPool = up4.applicationIDsPool[1:]

	up4.applicationIDs[app] = allocated

	return allocated, nil
}

// FIXME: SDFAB-960
//nolint:unused
func (up4 *UP4) releaseInternalApplicationID(appFilter applicationFilter) {
	app := application{
		appIP:     appFilter.srcIP,
		appL4Port: appFilter.srcPortRange,
		appProto:  appFilter.proto,
	}

	allocated, exists := up4.applicationIDs[app]
	if exists {
		delete(up4.applicationIDs, app)
		up4.applicationIDsPool = append(up4.applicationIDsPool, allocated)
	}
}

func (up4 *UP4) getOrAllocateInternalApplicationID(pdr pdr) (uint8, error) {
	var app application
	if pdr.IsUplink() {
		app = application{
			appIP:     pdr.appFilter.dstIP,
			appL4Port: pdr.appFilter.dstPortRange,
		}
	} else if pdr.IsDownlink() {
		app = application{
			appIP:     pdr.appFilter.srcIP,
			appL4Port: pdr.appFilter.srcPortRange,
		}
	}

	app.appProto = pdr.appFilter.proto

	if allocated, exists := up4.applicationIDs[app]; exists {
		return allocated, nil
	}

	newAppID, err := up4.allocateInternalApplicationID(app)
	if err != nil {
		return 0, err
	}

	return newAppID, nil
}

func (up4 *UP4) updateUEAddrAndFSEIDMappings(pdr pdr) {
	if !pdr.IsDownlink() {
		return
	}

	// update both maps in one shot
	up4.ueAddrToFSEID[pdr.ueAddress], up4.fseidToUEAddr[pdr.fseID] = pdr.fseID, pdr.ueAddress
}

func (up4 *UP4) removeUeAddrAndFSEIDMappings(pdr pdr) {
	if !pdr.IsDownlink() {
		return
	}

	delete(up4.ueAddrToFSEID, pdr.ueAddress)
	delete(up4.fseidToUEAddr, pdr.fseID)
}

func (up4 *UP4) updateTunnelPeersBasedOnFARs(fars []far) error {
	for _, far := range fars {
		logger := log.WithFields(log.Fields{
			"far": far,
		})
		// downlink FAR with tunnel params that does encapsulation
		if far.Forwards() && far.dstIntf == ie.DstInterfaceAccess && far.tunnelTEID != 0 {
			if err := up4.addOrUpdateGTPTunnelPeer(far); err != nil {
				logger.Errorf("Failed to add or update GTP tunnel peer: %v", err)
				return err
			}
		}
	}

	return nil
}

func verifyPDR(pdr pdr) error {
	if pdr.precedence > math.MaxUint16 {
		return ErrUnsupported("precedence greater than 65535", pdr.precedence)
	}

	return nil
}

// modifyUP4ForwardingConfiguration builds P4Runtime table entries and
// inserts/modifies/removes table entries from UP4 device, according to methodType.
func (up4 *UP4) modifyUP4ForwardingConfiguration(pdrs []pdr, allFARs []far, methodType p4.Update_Type) error {
	for _, pdr := range pdrs {
		if err := verifyPDR(pdr); err != nil {
			return err
		}

		entriesToApply := make([]*p4.TableEntry, 0)

		pdrLog := log.WithFields(log.Fields{
			"pdr": pdr,
		})
		pdrLog.Debug("Installing P4 table entries for PDR")

		far, err := findRelatedFAR(pdr, allFARs)
		if err != nil {
			pdrLog.Warning("no related FAR for PDR found: ", err)
			return err
		}

		pdrLog = pdrLog.WithField("related FAR", far)
		pdrLog.Debug("Found related FAR for PDR")

		tunnelParams := tunnelParams{
			tunnelIP4Src: ip2int(up4.accessIP.IP),
			tunnelIP4Dst: far.tunnelIP4Dst,
			tunnelPort:   far.tunnelPort,
		}

		tunnelPeerID, exists := up4.tunnelPeerIDs[tunnelParams]
		if !exists && far.tunnelTEID != 0 {
			return ErrNotFoundWithParam("allocated GTP tunnel peer ID", "tunnel params", tunnelParams)
		}

		sessionsEntry, err := up4.p4RtTranslator.BuildSessionsTableEntry(pdr, tunnelPeerID, far.Buffers())
		if err != nil {
			return ErrOperationFailedWithReason("build P4rt table entry for Sessions table", err.Error())
		}

		entriesToApply = append(entriesToApply, sessionsEntry)

		if pdr.IsUplink() {
			ueAddr, exists := up4.fseidToUEAddr[pdr.fseID]
			if !exists {
				// this is only possible if a linked DL PDR was not provided in the same PFCP Establishment message
				log.Error("UE Address not found for uplink PDR, a linked DL PDR was not provided?")
				return ErrOperationFailedWithReason("adding UP4 entries", "UE Address not found for uplink PDR, a linked DL PDR was not provided?")
			}

			pdr.ueAddress = ueAddr
		}

		var applicationsEntry *p4.TableEntry

		// as a default value is installed if no application filtering rule exists
		var applicationID uint8 = DefaultApplicationID
		if !pdr.IsAppFilterEmpty() {
			applicationID, err = up4.getOrAllocateInternalApplicationID(pdr)
			if err != nil {
				pdrLog.Error("failed to get or allocate internal application ID")
				return err
			}
		}

		// TODO: the same app filter can be simultaneously used by another UE session. We cannot remove it.
		//  We should come up with a way to check if an app filter is still in use.
		if applicationID != 0 && methodType != p4.Update_DELETE {
			applicationsEntry, err = up4.p4RtTranslator.BuildApplicationsTableEntry(pdr, applicationID)
			if err != nil {
				return ErrOperationFailedWithReason("build P4rt table entry for Applications table", err.Error())
			}

			entriesToApply = append(entriesToApply, applicationsEntry)
		}

		// FIXME: get TC from QFI->TC mapping
		terminationsEntry, err := up4.p4RtTranslator.BuildTerminationsTableEntry(pdr, far, applicationID, uint8(0))
		if err != nil {
			return ErrOperationFailedWithReason("build P4rt table entry for Terminations table", err.Error())
		}

		entriesToApply = append(entriesToApply, terminationsEntry)

		pdrLog = pdrLog.WithFields(log.Fields{
			"entries":     entriesToApply,
			"method type": p4.Update_Type_name[int32(methodType)],
		})
		pdrLog.Debug("Applying table entries")

		err = up4.p4client.ApplyTableEntries(methodType, entriesToApply...)
		if err != nil {
			p4Error, ok := err.(*P4RuntimeError)
			if !ok {
				// not a P4Runtime error, returning err
				return ErrOperationFailedWithReason("applying table entries to UP4", err.Error())
			}

			for _, status := range p4Error.Get() {
				// ignore ALREADY_EXISTS or OK
				if status.GetCanonicalCode() == int32(codes.AlreadyExists) ||
					status.GetCanonicalCode() == int32(codes.OK) {
					continue
				}

				return ErrOperationFailedWithReason("applying table entries to UP4", p4Error.Error())
			}
		}
	}

	return nil
}

func (up4 *UP4) sendCreate(all PacketForwardingRules, updated PacketForwardingRules) error {
	for i := range updated.pdrs {
		val, err := getCounterVal(up4, preQosCounterID)
		if err != nil {
			log.Println("Counter id alloc failed ", err)
			return ErrOperationFailedWithReason("Counter ID allocation", err.Error())
		}

		updated.pdrs[i].ctrID = uint32(val)
	}

	for _, p := range updated.pdrs {
		up4.updateUEAddrAndFSEIDMappings(p)
	}

	if err := up4.updateTunnelPeersBasedOnFARs(updated.fars); err != nil {
		// TODO: revert operations (e.g. reset counter)
		return err
	}

	if err := up4.modifyUP4ForwardingConfiguration(all.pdrs, all.fars, p4.Update_INSERT); err != nil {
		// TODO: revert operations (e.g. reset counter)
		return err
	}

	return nil
}

func (up4 *UP4) sendUpdate(all PacketForwardingRules, updated PacketForwardingRules) error {
	// Update PDR IE might modify UE IP <-> F-SEID mappings
	for _, p := range updated.pdrs {
		up4.updateUEAddrAndFSEIDMappings(p)
	}

	if err := up4.updateTunnelPeersBasedOnFARs(updated.fars); err != nil {
		return err
	}

	if err := up4.modifyUP4ForwardingConfiguration(all.pdrs, all.fars, p4.Update_MODIFY); err != nil {
		return err
	}

	return nil
}

func (up4 *UP4) sendDelete(deleted PacketForwardingRules) error {
	for i := range deleted.pdrs {
		resetCounterVal(up4, preQosCounterID,
			uint64(deleted.pdrs[i].ctrID))
	}

	if err := up4.modifyUP4ForwardingConfiguration(deleted.pdrs, deleted.fars, p4.Update_DELETE); err != nil {
		return err
	}

	for _, p := range deleted.pdrs {
		up4.removeUeAddrAndFSEIDMappings(p)
	}

	return nil
}

func (up4 *UP4) sendMsgToUPF(method upfMsgType, all PacketForwardingRules, updated PacketForwardingRules) uint8 {
	if !up4.isConnected(nil) {
		log.Error("UP4 server not connected")
		return ie.CauseRequestRejected
	}

	up4Log := log.WithFields(log.Fields{
		"method-type":   method,
		"all":           all,
		"updated-rules": updated,
	})
	up4Log.Debug("Sending PFCP message to UP4..")

	var err error

	switch method {
	case upfMsgTypeAdd:
		err = up4.sendCreate(all, updated)
	case upfMsgTypeMod:
		err = up4.sendUpdate(all, updated)
	case upfMsgTypeDel:
		err = up4.sendDelete(all)
	default:
		// unknown upfMsgType
		return ie.CauseRequestRejected
	}

	if err != nil {
		up4Log.Errorf("failed to apply forwarding configuration to UP4: %v", err)
		return ie.CauseRequestRejected
	}

	return ie.CauseRequestAccepted
}
