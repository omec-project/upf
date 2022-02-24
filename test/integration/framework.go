// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"errors"
	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/omec-project/upf-epc/pkg/bessmock"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"net"
	"os"
	"testing"
	"time"
)

// this file should contain all the struct defs/constants used among different test cases.

const (
	EnvMode = "MODE"
	EnvFastpath = "FASTPATH"

	FastpathUP4 = "up4"
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
)

var (
	// ReaderElectionID use reader election ID so that pfcpiface doesn't loose mastership.
	ReaderElectionID = p4_v1.Uint128{High: 0, Low: 1}
)

var (
	pfcpClient *pfcpsim.PFCPClient
	// pfcpAgent instance is used only in the native mode
	pfcpAgent *pfcpiface.PFCPIface

	bessMock *bessmock.BESSMock
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
	QFI uint8

	sessMBR uint64
	sessGBR uint64

	// uplink/downlink GBR/MBR is always the same
	appMBR uint64
	appGBR uint64
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
}

type testContext struct {
	UPFBasedUeIPAllocation bool
}

type testCase struct {
	ctx      testContext
	input    *pfcpSessionData
	expected p4RtValues

	desc string
}

func init() {
	logrus.SetLevel(logrus.TraceLevel)
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

func IsConnectionOpen(host string, port string) bool {
	ln, err := net.Listen("udp", host+":"+port)
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

func waitForPFCPAgentToStart() error {
	timeout := time.After(5 * time.Second)
	ticker := time.Tick(500 * time.Millisecond)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker:
			if IsConnectionOpen("127.0.0.1", "8805") {
				return nil
			}
		}
	}
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

func setup(t *testing.T, configType uint32) {
	// TODO: we currently need to reset the DefaultRegisterer between tests, as some leave the
	// 		 the registry in a bad state. Use custom registries to avoid global state.
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	switch os.Getenv(EnvFastpath) {
	case FastpathBESS:
		bessMock = bessmock.NewBESSMock(":10514", "127.0.0.1")
		go func() {
			if err := bessMock.Run(); err != nil {
				panic(err)
			}
		}()
	}

	time.Sleep(3*time.Second)

	switch os.Getenv(EnvMode) {
	case ModeDocker:
		//providers.RunDockerCommandAttach("pfcpiface",
		//	fmt.Sprintf("/bin/pfcpiface -config /config/%s", pfcpAgentConfig))
	case ModeNative:
		pfcpAgent = pfcpiface.NewPFCPIface(GetConfig(os.Getenv(EnvFastpath), configType))
		go pfcpAgent.Run()
	default:
		t.Fatal("Unexpected test mode")
	}

	// wait for PFCP Agent to initialize, blocking
	err := waitForPFCPAgentToStart()
	require.NoErrorf(t, err, "failed to start PFCP Agent: %v", err)

	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	err = pfcpClient.ConnectN4("127.0.0.1")
	require.NoErrorf(t, err, "failed to connect to UPF")
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

	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}

	switch os.Getenv(EnvMode) {
	case ModeDocker:
		// kill pfcpiface process inside container
		_, _, _, err := providers.RunDockerExecCommand("pfcpiface", "killall -9 pfcpiface")
		require.NoError(t, err, "failed to kill pfcpiface process")
	case ModeNative:
		pfcpAgent.Stop()
	default:
		t.Fatal("Unexpected test mode")
	}

	switch os.Getenv(EnvFastpath) {
	case FastpathBESS:
		if bessMock != nil {
			bessMock.Stop()
		}
	}
}

func verifyEntries(t *testing.T, testdata *pfcpSessionData, expectedValues p4RtValues, afterModification bool) {
	switch os.Getenv(EnvFastpath) {
	case FastpathUP4:
		verifyEntries(t, testdata, expectedValues, afterModification)
	case FastpathBESS:
		// TODO: implement it
	}
}

func verifyNoEntries(t *testing.T, expectedValues p4RtValues) {
	switch os.Getenv(EnvFastpath) {
	case FastpathUP4:
		verifyNoP4RuntimeEntries(t, expectedValues)
	case FastpathBESS:
		// TODO: implement it
	}
}
