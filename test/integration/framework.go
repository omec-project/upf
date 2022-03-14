// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"encoding/json"
	"errors"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/omec-project/upf-epc/pkg/fake_bess"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"
)

// this file should contain all the struct defs/constants used among different test cases.

const (
	EnvMode     = "MODE"
	EnvFastpath = "FASTPATH"

	FastpathUP4  = "up4"
	FastpathBESS = "bess"

	ModeDocker = "docker"
	ModeNative = "native"

	defaultSliceID = 0

	defaultSDFFilter = "permit out udp from any to assigned 80-80"

	ueAddress    = "17.0.0.1"
	upfN3Address = "198.18.0.1"
	nodeBAddress = "198.18.0.10"

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8

	srcIfaceAccess = 0x1
	srcIfaceCore   = 0x2

	directionUplink   = 0x1
	directionDownlink = 0x2

	p4InfoPath       = "../../conf/p4/bin/p4info.txt"
	deviceConfigPath = "../../conf/p4/bin/bmv2.json"
)

type UEState uint8

const (
	// UEStateAttaching after PFCP Session Establishment is done, but before PFCP Session Modification.
	UEStateAttaching UEState = iota
	// UEStateAttached state after PFCP Session Modification is done.
	UEStateAttached
	// UEStateBuffering state after PFCP Session Modification with buffering flags is done.
	UEStateBuffering
)

var (
	// ReaderElectionID use reader election ID so that pfcpiface doesn't lose mastership.
	ReaderElectionID = p4_v1.Uint128{High: 0, Low: 1}
)

var (
	pfcpClient *pfcpsim.PFCPClient
	// pfcpAgent instance is used only in the native mode
	pfcpAgent *pfcpiface.PFCPIface

	bessFake *fake_bess.FakeBESS
)

type pfcpSessionData struct {
	sliceID uint8

	nbAddress    string
	ueAddress    string
	upfN3Address string

	sdfFilter string

	precedence uint32

	ulTEID uint32
	dlTEID uint32

	// QER-related fields
	sessQerID        uint32
	uplinkAppQerID   uint32
	downlinkAppQerID uint32
	// only single QFI is fine, QFI is passed in session QER, but not considered.
	QFI     uint8
	sessMBR uint64
	sessGBR uint64
	// uplink/downlink GBR/MBR is always the same
	appMBR       uint64
	appGBR       uint64
	ulGateClosed bool
	dlGateClosed bool
}

type portRange struct {
	low  uint16
	high uint16
}

type appFilter struct {
	proto        uint8
	appIP        net.IP
	appPrefixLen uint32
	appPort      portRange
}

type p4RtValues struct {
	tc           uint8
	ueAddress    string
	tunnelPeerID uint8
	appID        uint8
	appFilter    appFilter

	pdrs []*ie.IE
	fars []*ie.IE
	qers []*ie.IE
}

type testContext struct {
	UPFBasedUeIPAllocation bool
}

type testCase struct {
	ctx      testContext
	input    *pfcpSessionData
	expected p4RtValues

	desc string

	// modified by test cases only
	session *pfcpsim.PFCPSession
}

func init() {
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
}

func TimeBasedElectionId() p4_v1.Uint128 {
	now := time.Now()
	return p4_v1.Uint128{
		High: uint64(now.Unix()),
		Low:  uint64(now.UnixNano() % 1e9),
	}
}

func (af appFilter) isEmpty() bool {
	return af.proto == 0 && len(af.appIP) == 0 &&
		af.appPort.low == 0 && af.appPort.high == 0
}

func IsConnectionOpen(network string, host string, port string) bool {
	target := net.JoinHostPort(host, port)

	switch network {
	case "udp":
		ln, err := net.Listen(network, target)
		if err != nil {
			return true
		}
		ln.Close()
	case "tcp":
		conn, err := net.DialTimeout(network, target, time.Second*3)
		if err != nil {
			return false
		}

		if conn != nil {
			conn.Close()
			return true
		}
	}

	return false
}

func waitForPortOpen(net string, host string, port string) error {
	timeout := time.After(5 * time.Second)
	ticker := time.Tick(500 * time.Millisecond)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker:
			if IsConnectionOpen(net, host, port) {
				return nil
			}
		}
	}
}

// waitForPFCPAssociationSetup checks if PFCP Agent is started by trying to create PFCP association.
// It retries every 1.5 seconds (0.5 seconds of interval between tries + 1 seconds that PFCP Client waits for response).
func waitForPFCPAssociationSetup(pfcpClient *pfcpsim.PFCPClient) error {
	timeout := time.After(30 * time.Second)
	ticker := time.Tick(500 * time.Millisecond)

	// Decrease timeout to wait for PFCP responses.
	// This decreases time to wait for PFCP Agent to start.
	pfcpClient.SetPFCPResponseTimeout(1 * time.Second)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker:
			// each test case requires PFCP Association, so we don't teardown it once we notice it's established.
			if err := pfcpClient.SetupAssociation(); err == nil {
				return nil
			}
		}
	}
}

func waitForBESSFakeToStart() error {
	return waitForPortOpen("tcp", "127.0.0.1", "10514")
}

func isModeNative() bool {
	return os.Getenv(EnvMode) == ModeNative
}

func isModeDocker() bool {
	return os.Getenv(EnvMode) == ModeDocker
}

func isFastpathUP4() bool {
	return os.Getenv(EnvFastpath) == FastpathUP4
}

func isFastpathBESS() bool {
	return os.Getenv(EnvFastpath) == FastpathBESS
}

func initForwardingPipelineConfig() {
	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", ReaderElectionID)
	if err != nil {
		panic("Cannot init forwarding pipeline config: " + err.Error())
	}
	defer providers.DisconnectP4rt()

	_, err = p4rtClient.SetFwdPipe(deviceConfigPath, p4InfoPath, 0)
	if err != nil {
		panic("Cannot init forwarding pipeline config: " + err.Error())
	}
}

func setup(t *testing.T, configType uint32) {
	// TODO: we currently need to reset the DefaultRegisterer between tests, as some leave the
	// 		 the registry in a bad state. Use custom registries to avoid global state.
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	switch os.Getenv(EnvFastpath) {
	case FastpathBESS:
		bessFake = fake_bess.NewFakeBESS()
		go func() {
			if err := bessFake.Run(":10514"); err != nil {
				panic(err)
			}
		}()

		err := waitForBESSFakeToStart()
		require.NoErrorf(t, err, "failed to start BESS fake: %v", err)
	}

	switch os.Getenv(EnvMode) {
	case ModeDocker:
		jsonConf, _ := json.Marshal(GetConfig(os.Getenv(EnvFastpath), configType))
		err := ioutil.WriteFile("./infra/upf.json", jsonConf, os.ModePerm)
		require.NoError(t, err)
		providers.MustRunDockerCommandAttach("pfcpiface",
			"/bin/pfcpiface -config /config/upf.json")
	case ModeNative:
		pfcpAgent = pfcpiface.NewPFCPIface(GetConfig(os.Getenv(EnvFastpath), configType))
		go pfcpAgent.Run()
	default:
		t.Fatal("Unexpected test mode")
	}

	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	err := pfcpClient.ConnectN4("127.0.0.1")
	require.NoErrorf(t, err, "failed to connect to UPF")

	// wait for PFCP Agent to initialize, blocking
	err = waitForPFCPAssociationSetup(pfcpClient)
	require.NoErrorf(t, err, "failed to start PFCP Agent: %v", err)
}

func teardown(t *testing.T) {
	if isFastpathUP4() {
		// clear Tunnel Peers table
		// FIXME: Temporary solution. They should be cleared by pfcpiface, see SDFAB-960
		p4rtClient, _ := providers.ConnectP4rt("127.0.0.1:50001", TimeBasedElectionId())
		defer providers.DisconnectP4rt()
		entries, _ := p4rtClient.ReadTableEntryWildcard("PreQosPipe.tunnel_peers")
		for _, entry := range entries {
			p4rtClient.DeleteTableEntry(entry)
		}
	}

	if pfcpClient.IsAssociationAlive() {
		err := pfcpClient.TeardownAssociation()
		require.NoError(t, err)
	}

	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}

	switch os.Getenv(EnvMode) {
	case ModeDocker:
		// kill pfcpiface process inside container
		_, _, _, err := providers.RunDockerExecCommand("pfcpiface", "killall -9 pfcpiface")
		require.NoError(t, err, "failed to kill pfcpiface process")

		err = os.Remove("./infra/upf.json")
		require.NoError(t, err)
	case ModeNative:
		pfcpAgent.Stop()
	default:
		t.Fatal("Unexpected test mode")
	}

	switch os.Getenv(EnvFastpath) {
	case FastpathBESS:
		if bessFake != nil {
			bessFake.Stop()
		}
	}
}

func verifyEntries(t *testing.T, testdata *pfcpSessionData, expectedValues p4RtValues, ueState UEState) {
	switch os.Getenv(EnvFastpath) {
	case FastpathUP4:
		verifyP4RuntimeEntries(t, testdata, expectedValues, ueState)
	case FastpathBESS:
		verifyBessEntries(t, bessFake, testdata, expectedValues, ueState)
	}
}

func verifyNoEntries(t *testing.T, expectedValues p4RtValues) {
	switch os.Getenv(EnvFastpath) {
	case FastpathUP4:
		verifyNoP4RuntimeEntries(t, expectedValues)
	case FastpathBESS:
		verifyNoBessRuntimeEntries(t, bessFake)
	}
}
