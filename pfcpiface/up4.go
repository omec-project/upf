// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/connectivity"

	"github.com/omec-project/upf-epc/internal/p4constants"
	"google.golang.org/grpc/codes"

	p4 "github.com/p4lang/p4runtime/go/p4/v1"

	set "github.com/deckarep/golang-set"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

const (
	preQosCounterID = iota
	postQosCounterID

	// 253 base stations + 1 dbuf (fixed in UP4) + 1 reserved (fixed in P4 pipeline)
	maxGTPTunnelPeerIDs = 253
	maxApplicationIDs   = 254

	meterTypeApplication uint8 = 1
	meterTypeSession     uint8 = 2

	// DefaultQFI is set if no QER is sent by a control plane in PFCP messages.
	// QFI=9 is used as a default value, because many Aether configurations uses it as default.
	// TODO: we might want to make it configurable in future.
	DefaultQFI = 9
)

var (
	p4RtcServerIP   = flag.String("p4RtcServerIP", "", "P4 Server ip")
	p4RtcServerPort = flag.String("p4RtcServerPort", "", "P4 Server port")
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

// meterID is a tuple of F-SEID and QER ID.
// This structure guarantees that QER ID is unique per PFCP session.
type meterID struct {
	qerID uint32
	fseid uint64
}

type meter struct {
	meterType      uint8
	uplinkCellID   uint32
	downlinkCellID uint32
}

type UP4 struct {
	conf P4rtcInfo

	host            string
	deviceID        uint64
	timeout         uint32
	accessIP        *net.IPNet
	ueIPPool        *net.IPNet
	enableEndMarker bool

	p4client *P4rtClient

	connected bool
	// connectedMu guards R/W operations to connected status
	connectedMu sync.RWMutex

	initOnce sync.Once
	// tryConnectMu ensures a single re-connection try
	tryConnectMu sync.Mutex

	p4RtTranslator *P4rtTranslator

	// TODO: create UP4Store object and move these fields there
	counters           []counter
	tunnelPeerIDs      map[tunnelParams]uint8
	tunnelPeerIDsPool  []uint8
	applicationIDs     map[application]uint8
	applicationIDsPool []uint8

	// meters stores the mapping from <F-SEID; QER ID> -> P4 Meter Cell ID.
	// P4 Meter Cell ID is retrieved from appMeterCellIDsPool or sessMeterCellIDsPool,
	// depending on QER type (application/session).
	meters               map[meterID]meter
	appMeterCellIDsPool  set.Set
	sessMeterCellIDsPool set.Set

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

func (m meter) String() string {
	return fmt.Sprintf("Meter(type=%d, uplinkCellID=%d, downlinkCellID=%d)",
		m.meterType, m.uplinkCellID, m.downlinkCellID)
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

func (up4 *UP4) initCounter(counterID uint8, name string, counterSize uint64) {
	up4.counters[counterID].maxSize = counterSize
	up4.counters[counterID].counterID = uint64(counterID)

	log.WithFields(log.Fields{
		"counterID":      counterID,
		"name":           name,
		"max-size":       counterSize,
		"UP4 counter ID": counterID,
	}).Debug("Counter initialized successfully")
}

func resetCounterVal(p *UP4, counterID uint8, val uint64) {
	log.Println("delete counter val ", val)
	delete(p.counters[counterID].allocated, val)
}

func (up4 *UP4) getCounterVal(counterID uint8) (uint64, error) {
	/*
	   loop :
	      random counter generate
	      check allocated map
	      if not in map then return counter val.
	      if present continue
	      if loop reaches max break and fail.
	*/
	var val uint64

	ctr := &up4.counters[counterID]
	for i := 0; i < int(ctr.maxSize); i++ {
		rand.Seed(time.Now().UnixNano())

		val = uint64(rand.Intn(int(ctr.maxSize)-1) + 1) // #nosec G404
		if _, ok := ctr.allocated[val]; !ok {
			log.Debug("Counter index is not in allocated map, assigning: ", val)

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
	setupLog := log.WithFields(log.Fields{
		"P4Runtime server address": up4.host,
		"DeviceID":                 up4.deviceID,
	})
	setupLog.Debug("Trying to setup P4Rt channel")

	client, err := CreateChannel(up4.host, up4.deviceID)
	if err != nil {
		setupLog.Errorf("create channel failed: %v", err)
		return err
	}

	up4.p4client = client

	err = up4.p4client.GetForwardingPipelineConfig()
	if err != nil {
		return err
	}

	setupLog.Debug("P4Rt channel created")

	return nil
}

func (up4 *UP4) initAllCounters() {
	log.Debug("Initializing counters for UP4")

	counters := []uint32{
		p4constants.CounterPreQosPipePreQosCounter,
		p4constants.CounterPostQosPipePostQosCounter,
	}

	for _, counterID := range counters {
		counterName := p4constants.GetCounterIDToNameMap()[counterID]

		counterSize, err := up4.p4RtTranslator.getCounterSizeByID(counterID)
		if err != nil {
			log.Error(err)
		}

		switch counterID {
		case p4constants.CounterPreQosPipePreQosCounter:
			up4.initCounter(preQosCounterID, counterName, uint64(counterSize))
		case p4constants.CounterPostQosPipePostQosCounter:
			up4.initCounter(postQosCounterID, counterName, uint64(counterSize))
		}
	}
}

func (up4 *UP4) initMetersPools() {
	log.Debug("Initializing P4 Meters pools for UP4")

	meters := []uint32{
		p4constants.MeterPreQosPipeAppMeter,
		p4constants.MeterPreQosPipeSessionMeter,
	}

	for _, meterID := range meters {
		meterName := p4constants.GetMeterIDToNameMap()[meterID]

		meterSize, err := up4.p4RtTranslator.getMeterSizeByID(meterID)
		if err != nil {
			log.Errorf("Could not find meter size of %v", meterName)
		}

		switch meterID {
		case p4constants.MeterPreQosPipeAppMeter:
			up4.appMeterCellIDsPool = set.NewSet()
			for i := 1; i < int(meterSize); i++ {
				up4.appMeterCellIDsPool.Add(uint32(i))
			}

			log.Trace("Application meter IDs pool initialized: ", up4.appMeterCellIDsPool.String())
		case p4constants.MeterPreQosPipeSessionMeter:
			up4.sessMeterCellIDsPool = set.NewSet()
			for i := 1; i < int(meterSize); i++ {
				up4.sessMeterCellIDsPool.Add(uint32(i))
			}

			log.Trace("Session meter IDs pool initialized: ", up4.sessMeterCellIDsPool.String())
		}
	}

	log.WithFields(log.Fields{
		"applicationMeter pool size": up4.appMeterCellIDsPool.Cardinality(),
		"sessMeter pool size":        up4.sessMeterCellIDsPool.Cardinality(),
	}).Debug("P4 Meters pools initialized successfully")
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
// FIXME: the argument should be removed from fastpath API
func (up4 *UP4) isConnected(accessIP *net.IP) bool {
	up4.connectedMu.Lock()
	defer up4.connectedMu.Unlock()

	return up4.connected && up4.p4client != nil && up4.p4client.CheckStatus() == connectivity.Ready
}

func (up4 *UP4) setConnectedStatus(status bool) {
	up4.connectedMu.Lock()
	defer up4.connectedMu.Unlock()

	up4.connected = status
}

// TODO: rename it to initUPF()
func (up4 *UP4) setUpfInfo(u *upf, conf *Conf) {
	log.Println("setUpfInfo UP4")

	up4.conf = conf.P4rtcIface

	up4.accessIP = MustParseStrIP(conf.P4rtcIface.AccessIP)
	u.accessIP = up4.accessIP.IP

	log.Infof("AccessIP: %v", up4.accessIP)

	up4.ueIPPool = MustParseStrIP(conf.CPIface.UEIPPool)

	log.Infof("UE IP pool: %v", up4.ueIPPool)

	p4rtcServer := conf.P4rtcIface.P4rtcServer

	p4rtcPort := conf.P4rtcIface.P4rtcPort
	up4.reportNotifyChan = u.reportNotifyChan

	if *p4RtcServerIP != "" {
		p4rtcServer = *p4RtcServerIP
	}

	if *p4RtcServerPort != "" {
		p4rtcPort = *p4RtcServerPort
	}

	u.coreIP = net.ParseIP(net.IPv4zero.String())

	up4.host = p4rtcServer + ":" + p4rtcPort

	log.WithFields(log.Fields{
		"UP4 endpoint": up4.host,
	}).Info("UP4 endpoint configured")

	up4.deviceID = 1
	up4.timeout = 30
	up4.enableEndMarker = conf.EnableEndMarker
	up4.initTunnelPeerIDs()
	up4.initApplicationIDs()
	up4.meters = make(map[meterID]meter)
	up4.ueAddrToFSEID = make(map[uint32]uint64)
	up4.fseidToUEAddr = make(map[uint64]uint32)

	up4.counters = make([]counter, 2)
	for i := range up4.counters {
		// initialize allocated counters map
		up4.counters[i].allocated = make(map[uint64]uint64)
	}

	go up4.keepTryingToConnect()
}

func (up4 *UP4) tryConnect() error {
	up4.tryConnectMu.Lock()
	defer up4.tryConnectMu.Unlock()

	if up4.isConnected(nil) {
		return nil
	}

	err := up4.setupChannel()
	if err != nil {
		return err
	}

	err = up4.initialize()
	if err != nil {
		log.Fatalf("Failed to initialize UP4: %v", err)
	}

	up4.setConnectedStatus(true)

	return nil
}

func (up4 *UP4) keepTryingToConnect() {
	for {
		err := up4.tryConnect()
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		time.Sleep(120 * time.Second)
	}
}

func (up4 *UP4) clearTables() error {
	tableIDs := []uint32{
		p4constants.TablePreQosPipeSessionsUplink,
		p4constants.TablePreQosPipeSessionsDownlink,
		p4constants.TablePreQosPipeTerminationsUplink,
		p4constants.TablePreQosPipeTerminationsDownlink,
		p4constants.TablePreQosPipeTunnelPeers,
		p4constants.TablePreQosPipeInterfaces,
		p4constants.TablePreQosPipeApplications,
	}

	if err := up4.p4client.ClearTables(tableIDs); err != nil {
		return err
	}

	return nil
}

// initInterfaces initializes N3 address and UE pool in the interfaces table.
// By sending both entries in batch, we ensure that both should always exist in UP4.
func (up4 *UP4) initInterfaces() error {
	entries := make([]*p4.TableEntry, 0, 2)

	uePoolEntry, err := up4.p4RtTranslator.BuildInterfaceTableEntry(up4.ueIPPool, up4.conf.SliceID, true)
	if err != nil {
		return err
	}

	entries = append(entries, uePoolEntry)

	n3AddrEntry, err := up4.p4RtTranslator.BuildInterfaceTableEntry(up4.accessIP, up4.conf.SliceID, false)
	if err != nil {
		return err
	}

	entries = append(entries, n3AddrEntry)

	if err := up4.p4client.ApplyTableEntries(p4.Update_INSERT, entries...); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"N3 address": up4.accessIP,
		"ue pool":    up4.ueIPPool,
	}).Debug("N3 address and UE pool successfully initialized in the UP4 pipeline")

	return nil
}

func (up4 *UP4) listenToDDNs() {
	log.Info("Listening to Data Notifications from UP4..")

	for {
		if up4.isConnected(nil) {
			// blocking
			digestData := up4.p4client.GetNextDigestData()

			ueAddr := binary.BigEndian.Uint32(digestData)
			if fseid, exists := up4.ueAddrToFSEID[ueAddr]; exists {
				up4.reportNotifyChan <- fseid
			}
		}
	}
}

// initialize configures the UP4-related objects.
// A caller should ensure that P4Client is not nil and the P4Runtime channel is open.
func (up4 *UP4) initialize() error {
	up4.p4RtTranslator = newP4RtTranslator(up4.p4client.P4Info)

	err := up4.clearTables()
	if err != nil {
		log.Warningf("failed to clear tables: %v", err)
		return err
	}

	up4.initAllCounters()
	up4.initMetersPools()

	err = up4.initInterfaces()
	if err != nil {
		return ErrOperationFailedWithReason("Interfaces initialization", err.Error())
	}

	up4.initOnce.Do(func() {
		go up4.listenToDDNs()

		if up4.enableEndMarker {
			log.Println("Starting end marker loop")

			up4.endMarkerChan = make(chan []byte, 1024)
			go up4.endMarkerSendLoop()
		}
	})

	return nil
}

func (up4 *UP4) sendEndMarkers(endMarkerList *[][]byte) error {
	for _, eMarker := range *endMarkerList {
		up4.endMarkerChan <- eMarker
	}

	return nil
}

func (up4 *UP4) endMarkerSendLoop() {
	for outPacket := range up4.endMarkerChan {
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

func findRelatedApplicationQER(pdr pdr, qers []qer) (qer, error) {
	for _, qer := range qers {
		if len(pdr.qerIDList) != 0 {
			// if only 1 QER provided, it's an application QER
			// if 2 QERs provided, the first one is an application QER
			// if more than 2 QERs provided, TODO: not supported
			if pdr.qerIDList[0] == qer.qerID {
				return qer, nil
			}
		}
	}

	return qer{}, ErrNotFoundWithParam("related application QER for PDR", "PDR", pdr)
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

func (up4 *UP4) allocateAppMeterCellID() (uint32, error) {
	// pick from set
	allocated := up4.appMeterCellIDsPool.Pop()
	if allocated == nil {
		return 0, ErrOperationFailedWithReason("allocate Application Meter Cell ID",
			"no free AppMeter Cell IDs available")
	}

	log.WithFields(log.Fields{
		"allocated ID":       allocated,
		"number of free IDs": up4.appMeterCellIDsPool.Cardinality(),
	}).Debug("Application meter cell ID allocated")

	return allocated.(uint32), nil
}

func (up4 *UP4) releaseAppMeterCellID(allocated uint32) {
	if allocated == 0 {
		// 0 is not a valid cell ID
		return
	}

	up4.appMeterCellIDsPool.Add(allocated)

	log.WithFields(log.Fields{
		"released ID":        allocated,
		"number of free IDs": up4.appMeterCellIDsPool.Cardinality(),
	}).Debug("Application meter cell ID released")
}

func (up4 *UP4) allocateSessionMeterCellID() (uint32, error) {
	// pick from set
	allocated := up4.sessMeterCellIDsPool.Pop()
	if allocated == nil {
		return 0, ErrOperationFailedWithReason("allocate Session Meter Cell ID",
			"no free SessionMeter Cell IDs available")
	}

	log.WithFields(log.Fields{
		"allocated ID":       allocated,
		"number of free IDs": up4.sessMeterCellIDsPool.Cardinality(),
	}).Debug("Session meter cell ID allocated")

	return allocated.(uint32), nil
}

func (up4 *UP4) releaseSessionMeterCellID(allocated uint32) {
	if allocated == 0 {
		// 0 is not a valid cell ID
		return
	}

	up4.sessMeterCellIDsPool.Add(allocated)

	log.WithFields(log.Fields{
		"released ID":        allocated,
		"number of free IDs": up4.sessMeterCellIDsPool.Cardinality(),
	}).Debug("Session meter cell ID released")
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

func getMeterConfigurationFromQER(mbr uint64, gbr uint64) *p4.MeterConfig {
	defaultBurstDurationMs := 10
	logger := log.WithFields(log.Fields{
		"GBR (Kbps)":         gbr,
		"MBR (Kbps)":         mbr,
		"burstDuration (ms)": defaultBurstDurationMs,
	})
	logger.Debug("Converting GBR/MBR to P4 Meter configuration")

	// FIXME: calculate from rate once P4-UPF supports GBRs
	cbs := 0
	cir := 0

	pbs := calcBurstSizeFromRate(mbr, uint64(defaultBurstDurationMs))

	var pir uint64 = 0
	if mbr != 0 {
		/* MBR/GBR is received in Kilobits/sec.
		   CIR/PIR is sent in bytes */
		pir = maxUint64((mbr*1000)/8, uint64(cir))
	}

	logger = logger.WithFields(log.Fields{
		"CIR": cir,
		"CBS": cbs,
		"PIR": pir,
		"PBS": pbs,
	})
	logger.Debug("GBR/MBR has been converted to P4 Meter configuration")

	return &p4.MeterConfig{
		Cir:    int64(cir),
		Cburst: int64(cbs),
		Pir:    int64(pir),
		Pburst: int64(pbs),
	}
}

// configureApplicationMeter installs P4Runtime Meter Entries based on QoS configuration from QER.
// If bidirectional, this function allocates two independent meter cell IDs, one per direction.
func (up4 *UP4) configureApplicationMeter(q qer, bidirectional bool) (meter, error) {
	entries := make([]*p4.MeterEntry, 0)

	appMeter := meter{
		meterType:      meterTypeApplication,
		uplinkCellID:   0,
		downlinkCellID: 0,
	}

	uplinkCellID, err := up4.allocateAppMeterCellID()
	if err != nil {
		return meter{}, err
	}

	appMeter.uplinkCellID = uplinkCellID

	if bidirectional {
		cellID, err := up4.allocateAppMeterCellID()
		if err != nil {
			up4.releaseAppMeterCellID(uplinkCellID)
			return meter{}, err
		}

		appMeter.downlinkCellID = cellID
	} else {
		appMeter.downlinkCellID = uplinkCellID
	}

	releaseIDs := func() {
		if appMeter.uplinkCellID != 0 {
			up4.releaseSessionMeterCellID(appMeter.uplinkCellID)
		}

		if appMeter.downlinkCellID != appMeter.uplinkCellID {
			up4.releaseSessionMeterCellID(appMeter.downlinkCellID)
		}
	}

	if appMeter.uplinkCellID != 0 {
		meterConfig := getMeterConfigurationFromQER(q.ulMbr, q.ulGbr)

		meterEntry := up4.p4RtTranslator.BuildMeterEntry(p4constants.MeterPreQosPipeAppMeter, appMeter.uplinkCellID, meterConfig)

		entries = append(entries, meterEntry)
	}

	if appMeter.downlinkCellID != appMeter.uplinkCellID {
		meterConfig := getMeterConfigurationFromQER(q.dlMbr, q.dlGbr)

		meterEntry := up4.p4RtTranslator.BuildMeterEntry(p4constants.MeterPreQosPipeAppMeter, appMeter.downlinkCellID, meterConfig)

		entries = append(entries, meterEntry)
	}

	err = up4.p4client.ApplyMeterEntries(p4.Update_MODIFY, entries...)
	if err != nil {
		releaseIDs()
		return meter{}, err
	}

	return appMeter, nil
}

// configureSessionMeter installs two P4Runtime Meter Entries.
// Session QER is always bidirectional. Thus, this function always configures two independent cell IDs.
func (up4 *UP4) configureSessionMeter(q qer) (meter, error) {
	uplinkCellID, err := up4.allocateSessionMeterCellID()
	if err != nil {
		return meter{}, err
	}

	downlinkCellID, err := up4.allocateSessionMeterCellID()
	if err != nil {
		up4.releaseSessionMeterCellID(uplinkCellID)
		return meter{}, err
	}

	releaseIDs := func() {
		up4.releaseSessionMeterCellID(uplinkCellID)
		up4.releaseSessionMeterCellID(downlinkCellID)
	}

	logger := log.WithFields(log.Fields{
		"uplink cell ID":   uplinkCellID,
		"downlink cell ID": downlinkCellID,
		"qer":              q,
	})
	logger.Debug("Configuring Session Meter from QER")

	uplinkMeterConfig := getMeterConfigurationFromQER(q.ulMbr, q.ulGbr)
	uplinkMeterEntry := up4.p4RtTranslator.BuildMeterEntry(p4constants.MeterPreQosPipeSessionMeter, uplinkCellID, uplinkMeterConfig)

	downlinkMeterConfig := getMeterConfigurationFromQER(q.dlMbr, q.dlGbr)
	downlinkMeterEntry := up4.p4RtTranslator.BuildMeterEntry(p4constants.MeterPreQosPipeSessionMeter, downlinkCellID, downlinkMeterConfig)

	logger = logger.WithFields(log.Fields{
		"uplink meter entry":   uplinkMeterEntry,
		"downlink meter entry": downlinkMeterEntry,
	})
	logger.Debug("Installing P4 Meter entries")

	err = up4.p4client.ApplyMeterEntries(p4.Update_MODIFY, uplinkMeterEntry, downlinkMeterEntry)
	if err != nil {
		releaseIDs()
		return meter{}, err
	}

	logger.Debug("P4 Meter entries installed successfully")

	return meter{
		meterType:      meterTypeSession,
		uplinkCellID:   uplinkCellID,
		downlinkCellID: downlinkCellID,
	}, nil
}

func (up4 *UP4) configureMeters(qers []qer) error {
	log.WithFields(log.Fields{
		"qers": qers,
	}).Debug("Configuring P4 Meters based on QERs")

	for _, qer := range qers {
		logger := log.WithFields(log.Fields{
			"qer": qer,
		})
		logger.Debug("Configuring P4 Meter based on QER")

		// TODO: In case we have GBR QER, then we are going to program only app-level rate-limiting
		//  (i.e., SessQerId will always be 0).

		var (
			err   error
			meter meter
		)

		switch qer.qosLevel {
		case ApplicationQos:
			if len(qers) == 1 {
				// if only a single QER is created, the QER is marked as Application QER,
				// and all PDRs points to the same QER, which is not unique per direction.
				// Therefore, we have to configure bidirectional meter (two independent cells, one per direction).
				meter, err = up4.configureApplicationMeter(qer, true)
			} else {
				meter, err = up4.configureApplicationMeter(qer, false)
			}
		case SessionQos:
			meter, err = up4.configureSessionMeter(qer)
		default:
			// unknown, type of QER
			continue
		}

		if err != nil {
			return ErrOperationFailedWithReason("configure P4 Meter from QER", err.Error())
		}

		logger = logger.WithField("P4 meter", meter)
		logger.Debug("P4 meter successfully configured!")

		up4.meters[meterID{
			qerID: qer.qerID,
			fseid: qer.fseID,
		}] = meter
	}

	return nil
}

func verifyPDR(pdr pdr) error {
	if pdr.precedence > math.MaxUint16 {
		return ErrUnsupported("precedence greater than 65535", pdr.precedence)
	}

	return nil
}

func (up4 *UP4) resetMeter(meterID uint32, meter meter) {
	entries := make([]*p4.MeterEntry, 0, 2)

	entry := &p4.MeterEntry{
		MeterId: meterID,
		Index:   &p4.Index{Index: int64(meter.uplinkCellID)},
	}

	entries = append(entries, entry)

	if meter.downlinkCellID != meter.uplinkCellID {
		entry := &p4.MeterEntry{
			MeterId: meterID,
			Index:   &p4.Index{Index: int64(meter.downlinkCellID)},
		}
		entries = append(entries, entry)
	}

	err := up4.p4client.ApplyMeterEntries(p4.Update_MODIFY, entries...)
	if err != nil {
		log.Errorf("Failed to reset %v meter entries: %v", p4constants.GetMeterIDToNameMap()[meterID], err)
	}
}

func (up4 *UP4) resetMeters(qers []qer) {
	log.WithFields(log.Fields{
		"qers": qers,
	}).Debug("Resetting P4 Meters")

	for _, qer := range qers {
		logger := log.WithFields(log.Fields{
			"qer": qer,
		})
		logger.Debug("Resetting P4 Meter")

		meter, exists := up4.meters[meterID{
			qerID: qer.qerID,
			fseid: qer.fseID,
		}]
		if !exists {
			logger.Error("P4 meter for QER ID not found, cannot reset!")
			continue
		}

		if meter.meterType == meterTypeApplication {
			up4.resetMeter(p4constants.MeterPreQosPipeAppMeter, meter)
			up4.releaseAppMeterCellID(meter.uplinkCellID)

			if meter.downlinkCellID != meter.uplinkCellID {
				up4.releaseAppMeterCellID(meter.downlinkCellID)
			}
		} else if meter.meterType == meterTypeSession {
			up4.resetMeter(p4constants.MeterPreQosPipeSessionMeter, meter)
			up4.releaseSessionMeterCellID(meter.uplinkCellID)
			up4.releaseSessionMeterCellID(meter.downlinkCellID)
		}

		logger = logger.WithField("P4 meter", meter)
		logger.Debug("Removing P4 meter from allocated meters pool")

		delete(up4.meters, meterID{
			qerID: qer.qerID,
			fseid: qer.fseID,
		})
	}
}

// modifyUP4ForwardingConfiguration builds P4Runtime table entries and
// inserts/modifies/removes table entries from UP4 device, according to methodType.
func (up4 *UP4) modifyUP4ForwardingConfiguration(pdrs []pdr, allFARs []far, qers []qer, methodType p4.Update_Type) error {
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

		var sessMeter = meter{meterTypeSession, 0, 0}
		if len(pdr.qerIDList) == 2 {
			// if 2 QERs are provided, the second one is Session QER
			sessMeter = up4.meters[meterID{
				qerID: pdr.qerIDList[1],
				fseid: pdr.fseID,
			}]
			pdrLog.Debug("Application meter found for PDR: ", sessMeter)
		} // else: if only 1 QER provided, set sessMeterIdx to 0, and use only per-app metering

		sessionsEntry, err := up4.p4RtTranslator.BuildSessionsTableEntry(pdr, sessMeter, tunnelPeerID, far.Buffers())
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
			applicationsEntry, err = up4.p4RtTranslator.BuildApplicationsTableEntry(pdr, up4.conf.SliceID, applicationID)
			if err != nil {
				return ErrOperationFailedWithReason("build P4rt table entry for Applications table", err.Error())
			}

			entriesToApply = append(entriesToApply, applicationsEntry)
		}

		var appMeter = meter{meterTypeApplication, 0, 0}
		if len(pdr.qerIDList) != 0 {
			// if only 1 QER provided, it's an application QER
			// if 2 QERs provided, the first one is an application QER
			// if more than 2 QERs provided, TODO: not supported
			appMeter = up4.meters[meterID{
				qerID: pdr.qerIDList[0],
				fseid: pdr.fseID,
			}]
			pdrLog.Debug("Application meter found for PDR: ", appMeter)
		}

		var qfi uint8 = DefaultQFI

		relatedQER, err := findRelatedApplicationQER(pdr, qers)
		if err != nil {
			pdrLog.Warning(err)
		} else {
			pdrLog.Debug("Related QER found for PDR: ", relatedQER)
			qfi = relatedQER.qfi
		}

		tc, exists := up4.conf.QFIToTC[relatedQER.qfi]
		if !exists {
			tc = NoTC
		}

		terminationsEntry, err := up4.p4RtTranslator.BuildTerminationsTableEntry(pdr, appMeter, far,
			applicationID, qfi, tc, relatedQER)
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
		val, err := up4.getCounterVal(preQosCounterID)
		if err != nil {
			return ErrOperationFailedWithReason("Counter ID allocation", err.Error())
		}

		all.pdrs[i].ctrID = uint32(val)
	}

	for _, p := range updated.pdrs {
		up4.updateUEAddrAndFSEIDMappings(p)
	}

	if err := up4.configureMeters(updated.qers); err != nil {
		return err
	}

	if err := up4.updateTunnelPeersBasedOnFARs(updated.fars); err != nil {
		// TODO: revert operations (e.g. reset counter)
		return err
	}

	if err := up4.modifyUP4ForwardingConfiguration(all.pdrs, all.fars, all.qers, p4.Update_INSERT); err != nil {
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

	if err := up4.modifyUP4ForwardingConfiguration(all.pdrs, all.fars, all.qers, p4.Update_MODIFY); err != nil {
		return err
	}

	return nil
}

func (up4 *UP4) sendDelete(deleted PacketForwardingRules) error {
	for i := range deleted.pdrs {
		resetCounterVal(up4, preQosCounterID,
			uint64(deleted.pdrs[i].ctrID))
	}

	if err := up4.modifyUP4ForwardingConfiguration(deleted.pdrs, deleted.fars, deleted.qers, p4.Update_DELETE); err != nil {
		return err
	}

	up4.resetMeters(deleted.qers)

	for _, p := range deleted.pdrs {
		up4.removeUeAddrAndFSEIDMappings(p)
	}

	return nil
}

func (up4 *UP4) sendMsgToUPF(method upfMsgType, all PacketForwardingRules, updated PacketForwardingRules) uint8 {
	err := up4.tryConnect()
	if err != nil {
		log.Error("UP4 server not connected")
		return ie.CauseRequestRejected
	}

	up4Log := log.WithFields(log.Fields{
		"method-type":   method,
		"all":           all,
		"updated-rules": updated,
	})
	up4Log.Debug("Sending PFCP message to UP4..")

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
