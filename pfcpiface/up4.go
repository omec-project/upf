// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
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

// internalAppReference <F-SEID (UE session); PDR-ID> pair that
// uniquely identifies application filter among different PDR IEs of the same UE session.
type internalAppReference struct {
	fseid uint64
	pdrID uint32
}

type internalApp struct {
	id uint8
	// usedBy keeps track of <F-SEID (UE session); PDR-ID> pairs using this application filter.
	usedBy set.Set
}

type up4ApplicationFilter struct {
	appIP     uint32
	appL4Port portRange
	appProto  uint8
}

type counter struct {
	maxSize        uint64
	counterID      uint64
	counterIDsPool set.Set
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

// tnlPeerReference <F-SEID (UE session); FAR-ID> pair that
// uniquely identifies tunnel peer among different FAR IEs of the same UE session.
type tnlPeerReference struct {
	fseid uint64
	farID uint32
}

type tunnelPeer struct {
	id uint8
	// usedBy keeps track of <F-SEID (UE session); FAR-ID> pairs using this tunnel peer.
	usedBy set.Set
}

func (t tunnelPeer) String() string {
	return fmt.Sprintf("TunnelPeer{id=%d, usedBy=%d, referencing F-SEIDs=%v}",
		t.id, t.usedBy.Cardinality(), t.usedBy)
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
	counters []counter
	// tunnelPeerMu guards concurrent R/W access to tunnel peers,
	// as tunnel peers are likely to be shared between different UE sessions.
	tunnelPeerMu       sync.Mutex
	tunnelPeerIDs      map[tunnelParams]tunnelPeer
	tunnelPeerIDsPool  []uint8
	applicationMu      sync.Mutex
	applicationIDs     map[up4ApplicationFilter]internalApp
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

func toUP4ApplicationFilter(p pdr) up4ApplicationFilter {
	var appFilter up4ApplicationFilter
	if p.IsUplink() {
		appFilter = up4ApplicationFilter{
			appIP:     p.appFilter.dstIP,
			appL4Port: p.appFilter.dstPortRange,
		}
	} else if p.IsDownlink() {
		appFilter = up4ApplicationFilter{
			appIP:     p.appFilter.srcIP,
			appL4Port: p.appFilter.srcPortRange,
		}
	}

	appFilter.appProto = p.appFilter.proto

	return appFilter
}

func (m meter) String() string {
	return fmt.Sprintf("Meter(type=%d, uplinkCellID=%d, downlinkCellID=%d)",
		m.meterType, m.uplinkCellID, m.downlinkCellID)
}

func (up4 *UP4) AddSliceInfo(sliceInfo *SliceInfo) error {
	//FIXME: UP4 currently supports a single slice meter rate common between UL and DL traffic. For this reason, we
	//  configure the meter with the largest slice MBR between UL and DL.
	err := up4.tryConnect()
	if err != nil {
		log.Error("UP4 server not connected")
		return ErrOperationFailedWithReason("addSliceInfo", "data plane is not connected")
	}

	var sliceMbr, sliceBurstBytes uint64
	if sliceInfo.uplinkMbr > sliceInfo.downlinkMbr {
		sliceMbr = sliceInfo.uplinkMbr
		sliceBurstBytes = sliceInfo.ulBurstBytes
	} else {
		sliceMbr = sliceInfo.downlinkMbr
		sliceBurstBytes = sliceInfo.dlBurstBytes
	}

	meterCellId, err := GetSliceTCMeterIndex(up4.conf.SliceID, up4.conf.DefaultTC)
	if err != nil {
		return err
	}

	meterConfig := p4.MeterConfig{
		Cir:    int64(0),
		Cburst: int64(0),
		Pir:    int64(sliceMbr),
		Pburst: int64(sliceBurstBytes),
	}
	sliceMeterEntry := up4.p4RtTranslator.BuildMeterEntry(p4constants.MeterPreQosPipeSliceTcMeter, uint32(meterCellId), &meterConfig)

	log.WithFields(log.Fields{
		"Slice meter entry": sliceMeterEntry,
	}).Debug("Installing slice P4 Meter entry")

	err = up4.p4client.ApplyMeterEntries(p4.Update_MODIFY, sliceMeterEntry)

	if err != nil {
		return err
	}

	return nil
}

func (up4 *UP4) SummaryLatencyJitter(uc *upfCollector, ch chan<- prometheus.Metric) {
}

func (up4 *UP4) SessionStats(*PfcpNodeCollector, chan<- prometheus.Metric) error {
	return nil
}

func (up4 *UP4) PortStats(uc *upfCollector, ch chan<- prometheus.Metric) {
}

func (up4 *UP4) initCounter(counterID uint8, name string, counterSize uint64) {
	up4.counters[counterID].maxSize = counterSize
	up4.counters[counterID].counterID = uint64(counterID)
	up4.counters[counterID].counterIDsPool = set.NewSet()

	for i := uint64(0); i < up4.counters[counterID].maxSize; i++ {
		up4.counters[counterID].counterIDsPool.Add(i)
	}

	log.WithFields(log.Fields{
		"counterID":      counterID,
		"name":           name,
		"max-size":       counterSize,
		"UP4 counter ID": counterID,
	}).Debug("Counter initialized successfully")
}

func (up4 *UP4) releaseCounterID(p4counterID uint8, val uint64) {
	log.Println("delete counter val ", val)
	up4.counters[p4counterID].counterIDsPool.Add(val)
}

func (up4 *UP4) allocateCounterID(p4counterID uint8) (uint64, error) {
	if up4.counters[p4counterID].counterIDsPool.Cardinality() == 0 {
		return 0, ErrOperationFailedWithReason("allocate Counter ID",
			"no free Counter IDs available")
	}

	allocated := up4.counters[p4counterID].counterIDsPool.Pop()

	if allocated == nil {
		return 0, ErrOperationFailedWithReason("allocate Counter ID",
			"no free Counter IDs available")
	}

	return allocated.(uint64), nil
}

func (up4 *UP4) Exit() {
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

	up4.p4RtTranslator = newP4RtTranslator(up4.p4client.P4Info)

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
	up4.tunnelPeerIDs = make(map[tunnelParams]tunnelPeer)
	// a simple queue storing available tunnel peer IDs
	// 0 is reserved;
	// 1 is reserved for dbuf
	up4.tunnelPeerIDsPool = make([]uint8, 0, maxGTPTunnelPeerIDs)

	for i := 2; i < maxGTPTunnelPeerIDs+2; i++ {
		up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, uint8(i))
	}
}

func (up4 *UP4) initApplicationIDs() {
	up4.applicationIDs = make(map[up4ApplicationFilter]internalApp)
	// a simple queue storing available application IDs
	// 0 is reserved;
	up4.applicationIDsPool = make([]uint8, 0, maxApplicationIDs)

	for i := 1; i < maxApplicationIDs+1; i++ {
		up4.applicationIDsPool = append(up4.applicationIDsPool, uint8(i))
	}
}

// This function ensures that PFCP Agent is connected to UP4.
// Returns true if the connection is already established.
// FIXME: the argument should be removed from datapath API
func (up4 *UP4) IsConnected(accessIP *net.IP) bool {
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
func (up4 *UP4) SetUpfInfo(u *upf, conf *Conf) {
	log.Println("SetUpfInfo UP4")

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

	go up4.keepTryingToConnect()
}

func (up4 *UP4) tryConnect() error {
	up4.tryConnectMu.Lock()
	defer up4.tryConnectMu.Unlock()

	if up4.IsConnected(nil) {
		return nil
	}

	// datapath state should be cleared & initialized if P4Rt connection or ForwardingConfig is not setup yet.
	shouldClearAndInitialize := up4.p4client == nil || up4.p4client.P4Info == nil

	err := up4.setupChannel()
	if err != nil {
		log.Errorf("Failed to setup UP4 channel: %v", err)
		return err
	}

	err = up4.initialize(shouldClearAndInitialize)
	if err != nil {
		log.Errorf("Failed to initialize UP4: %v", err)
		return err
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

	notifier := NewDownlinkDataNotifier(up4.reportNotifyChan, 20*time.Second)

	for {
		if up4.IsConnected(nil) {
			// blocking
			digestData := up4.p4client.GetNextDigestData()

			ueAddr := binary.BigEndian.Uint32(digestData)
			if fseid, exists := up4.ueAddrToFSEID[ueAddr]; exists {
				notifier.Notify(fseid)
			}
		}
	}
}

func (up4 *UP4) clearDatapathState() error {
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

	return nil
}

// initialize configures the UP4-related objects.
// A caller should ensure that P4Client is not nil and the P4Runtime channel is open.
func (up4 *UP4) initialize(shouldClear bool) error {
	// always clear datapath state at startup or
	// on UP4 datapath restart if ClearStateOnRestart is enabled.
	if shouldClear || up4.conf.ClearStateOnRestart {
		if err := up4.clearDatapathState(); err != nil {
			return err
		}
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

func (up4 *UP4) SendEndMarkers(endMarkerList *[][]byte) error {
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
func (up4 *UP4) unsafeAllocateGTPTunnelPeerID() (uint8, error) {
	if len(up4.tunnelPeerIDsPool) == 0 {
		return 0, ErrOperationFailedWithReason("allocate GTP Tunnel Peer ID",
			"no free tunnel peer IDs available")
	}

	// pick top from queue
	allocated := up4.tunnelPeerIDsPool[0]
	up4.tunnelPeerIDsPool = up4.tunnelPeerIDsPool[1:]

	log.WithFields(log.Fields{
		"ID":   allocated,
		"pool": up4.tunnelPeerIDsPool,
	}).Trace("Tunnel peer ID")

	return allocated, nil
}

func (up4 *UP4) unsafeReleaseAllocatedGTPTunnelPeer(tunnelParams tunnelParams) {
	allocated, exists := up4.tunnelPeerIDs[tunnelParams]
	if exists {
		delete(up4.tunnelPeerIDs, tunnelParams)
		up4.tunnelPeerIDsPool = append(up4.tunnelPeerIDsPool, allocated.id)

		log.WithFields(log.Fields{
			"tunnel params": tunnelParams,
			"tunnel peer":   allocated,
			"pool":          up4.tunnelPeerIDsPool,
		}).Trace("Tunnel peer ID released")
	}
}

func (up4 *UP4) getGTPTunnelPeer(tnlParams tunnelParams) (tunnelPeer, bool) {
	up4.tunnelPeerMu.Lock()
	defer up4.tunnelPeerMu.Unlock()

	tnlPeer, exists := up4.tunnelPeerIDs[tnlParams]

	return tnlPeer, exists
}

func (up4 *UP4) addOrUpdateGTPTunnelPeer(far far) error {
	up4.tunnelPeerMu.Lock()
	defer up4.tunnelPeerMu.Unlock()

	var tnlPeer tunnelPeer

	methodType := p4.Update_MODIFY
	tunnelParams := tunnelParams{
		tunnelIP4Src: ip2int(up4.accessIP.IP),
		tunnelIP4Dst: far.tunnelIP4Dst,
		tunnelPort:   far.tunnelPort,
	}

	tnlPeer, exists := up4.tunnelPeerIDs[tunnelParams]
	if !exists {
		newID, err := up4.unsafeAllocateGTPTunnelPeerID()
		if err != nil {
			return err
		}

		tnlPeer = tunnelPeer{
			id: newID,
			usedBy: set.NewSet(tnlPeerReference{
				far.fseID, far.farID,
			}),
		}

		methodType = p4.Update_INSERT
	} else {
		// tunnel peer already exists.
		// since we use Set to keep track of tunnel peers in use,
		// it will not be added to the set if tunnel peer was already created for this UE session.
		tnlPeer.usedBy.Add(tnlPeerReference{
			far.fseID, far.farID,
		})
	}

	releaseTnlPeerID := func() {
		if !exists {
			up4.unsafeReleaseAllocatedGTPTunnelPeer(tunnelParams)
		}
	}

	gtpTunnelPeerEntry, err := up4.p4RtTranslator.BuildGTPTunnelPeerTableEntry(tnlPeer.id, tunnelParams)
	if err != nil {
		releaseTnlPeerID()
		return err
	}

	if err := up4.p4client.ApplyTableEntries(methodType, gtpTunnelPeerEntry); err != nil {
		releaseTnlPeerID()
		return err
	}

	up4.tunnelPeerIDs[tunnelParams] = tnlPeer

	return nil
}

func (up4 *UP4) removeGTPTunnelPeer(far far) {
	up4.tunnelPeerMu.Lock()
	defer up4.tunnelPeerMu.Unlock()

	removeLog := log.WithFields(log.Fields{
		"far": far,
	})
	tunnelParams := tunnelParams{
		tunnelIP4Src: ip2int(up4.accessIP.IP),
		tunnelIP4Dst: far.tunnelIP4Dst,
		tunnelPort:   far.tunnelPort,
	}

	tnlPeer, exists := up4.tunnelPeerIDs[tunnelParams]
	if !exists {
		removeLog.WithField(
			"tunnel-params", tunnelParams).Warn("GTP tunnel peer ID not found for tunnel params")
		return
	}

	removeLog.WithField("tunnel-peer", tnlPeer)

	removeLog.Debug("Found GTP tunnel peer for tunnel params")

	tnlPeer.usedBy.Remove(tnlPeerReference{
		far.fseID, far.farID,
	})

	if tnlPeer.usedBy.Cardinality() != 0 {
		removeLog.Debug("GTP tunnel peer was about to be removed, but it's in use by other UE session.")
		return
	}

	gtpTunnelPeerEntry, err := up4.p4RtTranslator.BuildGTPTunnelPeerTableEntry(tnlPeer.id, tunnelParams)
	if err != nil {
		removeLog.Error("failed to build GTP tunnel peer entry to remove")
		return
	}

	removeLog.Debug("Removing GTP Tunnel Peer ID")

	if err := up4.p4client.ApplyTableEntries(p4.Update_DELETE, gtpTunnelPeerEntry); err != nil {
		removeLog.Error("failed to remove GTP tunnel peer")
	}

	up4.unsafeReleaseAllocatedGTPTunnelPeer(tunnelParams)
}

// Returns error if we reach maximum supported Application IDs.
func (up4 *UP4) unsafeAllocateInternalApplicationID() (uint8, error) {
	if len(up4.applicationIDsPool) == 0 {
		return 0, ErrOperationFailedWithReason("allocate Application ID",
			"no free application IDs available")
	}

	// pick top from queue
	allocated := up4.applicationIDsPool[0]
	up4.applicationIDsPool = up4.applicationIDsPool[1:]

	return allocated, nil
}

func (up4 *UP4) unsafeReleaseInternalApplicationID(appFilter up4ApplicationFilter) {
	allocated, exists := up4.applicationIDs[appFilter]
	if exists {
		up4.applicationIDsPool = append(up4.applicationIDsPool, allocated.id)
		delete(up4.applicationIDs, appFilter)
	}
}

func (up4 *UP4) addInternalApplicationIDAndGetP4rtEntry(pdr pdr) (*p4.TableEntry, uint8, error) {
	up4.applicationMu.Lock()
	defer up4.applicationMu.Unlock()

	appFilter := toUP4ApplicationFilter(pdr)
	if up4Application, exists := up4.applicationIDs[appFilter]; exists {
		// application already exists, increment 'usedBy'.
		// since we use Set usedBy will not be incremented if
		// application was already created for this UE session + PDR ID.
		up4Application.usedBy.Add(internalAppReference{
			pdr.fseID, pdr.pdrID,
		})

		return nil, up4Application.id, nil
	}

	newAppID, err := up4.unsafeAllocateInternalApplicationID()
	if err != nil {
		return nil, 0, err
	}

	up4Application := internalApp{
		id: newAppID,
		usedBy: set.NewSet(internalAppReference{
			pdr.fseID, pdr.pdrID,
		}),
	}

	applicationsEntry, err := up4.p4RtTranslator.BuildApplicationsTableEntry(pdr, up4.conf.SliceID, newAppID)
	if err != nil {
		up4.unsafeReleaseInternalApplicationID(appFilter)
		return nil, 0, ErrOperationFailedWithReason("build P4rt table entry for Applications table", err.Error())
	}

	up4.applicationIDs[appFilter] = up4Application

	return applicationsEntry, up4Application.id, nil
}

func (up4 *UP4) removeInternalApplicationIDAndGetP4rtEntry(pdr pdr) (*p4.TableEntry, uint8) {
	up4.applicationMu.Lock()
	defer up4.applicationMu.Unlock()

	appFilter := toUP4ApplicationFilter(pdr)

	internalApp, exists := up4.applicationIDs[appFilter]
	if !exists {
		return nil, 0
	}

	internalApp.usedBy.Remove(internalAppReference{
		pdr.fseID, pdr.pdrID,
	})

	if internalApp.usedBy.Cardinality() != 0 {
		return nil, internalApp.id
	}

	applicationsEntry, err := up4.p4RtTranslator.BuildApplicationsTableEntry(pdr, up4.conf.SliceID, internalApp.id)
	if err != nil {
		return nil, internalApp.id
	}

	up4.unsafeReleaseInternalApplicationID(appFilter)

	return applicationsEntry, internalApp.id
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
	if pdr.IsUplink() {
		return
	}

	// update both maps in one shot
	up4.ueAddrToFSEID[pdr.ueAddress], up4.fseidToUEAddr[pdr.fseID] = pdr.fseID, pdr.ueAddress
}

func (up4 *UP4) removeUeAddrAndFSEIDMappings(pdr pdr) {
	if pdr.IsUplink() {
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

func (up4 *UP4) resetCounter(pdr pdr) error {
	builderLog := log.WithFields(log.Fields{
		"Cell ID": pdr.ctrID,
		"PDR ID":  pdr.pdrID,
		"F-SEID":  pdr.fseID,
	})
	builderLog.Debug("Clearing Counter cells")

	resetValue := &p4.CounterData{
		ByteCount:   0,
		PacketCount: 0,
	}

	cntrIndex := &p4.Index{Index: int64(pdr.ctrID)}

	ingressCntrEntry := &p4.CounterEntry{
		CounterId: p4constants.CounterPreQosPipePreQosCounter,
		Index:     cntrIndex,
		Data:      resetValue,
	}

	egressCntrEntry := &p4.CounterEntry{
		CounterId: p4constants.CounterPostQosPipePostQosCounter,
		Index:     cntrIndex,
		Data:      resetValue,
	}

	updates := []*p4.Update{
		{
			Type: p4.Update_MODIFY,
			Entity: &p4.Entity{
				Entity: &p4.Entity_CounterEntry{CounterEntry: ingressCntrEntry},
			},
		},
		{
			Type: p4.Update_MODIFY,
			Entity: &p4.Entity{
				Entity: &p4.Entity_CounterEntry{CounterEntry: egressCntrEntry},
			},
		},
	}

	return up4.p4client.WriteBatchReq(updates)
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

		tunnelPeerID, exists := up4.getGTPTunnelPeer(tunnelParams)
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

		sessionsEntry, err := up4.p4RtTranslator.BuildSessionsTableEntry(pdr, sessMeter, tunnelPeerID.id, far.Buffers())
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

		// as a default value is installed if no application filtering rule exists
		var applicationID uint8 = DefaultApplicationID

		if !pdr.IsAppFilterEmpty() {
			if methodType != p4.Update_DELETE {
				if entry, appID, err := up4.addInternalApplicationIDAndGetP4rtEntry(pdr); err == nil {
					if entry != nil {
						entriesToApply = append(entriesToApply, entry)
					}

					applicationID = appID
				}
			} else {
				entry, appID := up4.removeInternalApplicationIDAndGetP4rtEntry(pdr)
				if entry != nil {
					entriesToApply = append(entriesToApply, entry)
				}

				applicationID = appID
			}
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
			tc = up4.conf.DefaultTC
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
		val, err := up4.allocateCounterID(preQosCounterID)
		if err != nil {
			return ErrOperationFailedWithReason("Counter ID allocation", err.Error())
		}

		all.pdrs[i].ctrID = uint32(val)

		if err := up4.resetCounter(all.pdrs[i]); err != nil {
			return ErrOperationFailedWithReason("Reset Counters", err.Error())
		}
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
		up4.releaseCounterID(preQosCounterID,
			uint64(deleted.pdrs[i].ctrID))
	}

	if err := up4.modifyUP4ForwardingConfiguration(deleted.pdrs, deleted.fars, deleted.qers, p4.Update_DELETE); err != nil {
		return err
	}

	up4.resetMeters(deleted.qers)

	for _, f := range deleted.fars {
		up4.removeGTPTunnelPeer(f)
	}

	for _, p := range deleted.pdrs {
		up4.removeUeAddrAndFSEIDMappings(p)
	}

	return nil
}

func (up4 *UP4) SendMsgToUPF(method upfMsgType, all PacketForwardingRules, updated PacketForwardingRules) uint8 {
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
