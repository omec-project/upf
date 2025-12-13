// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/omec-project/pfcpsim/pkg/pfcpsim"
	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/omec-project/upf-epc/pkg/fake_bess"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wmnsk/go-pfcp/ie"
	"go.uber.org/zap"
)

// this file should contain all the struct defs/constants used among different test cases.

const (
	ConfigPath       = "/tmp/upf.jsonc"
	defaultSDFFilter = "permit out udp from any to assigned 80-80"

	ueAddress    = "17.0.0.1"
	upfN3Address = "198.18.0.1"
	nodeBAddress = "198.18.0.10"

	ActionForward uint8 = 0x2
	ActionDrop    uint8 = 0x1
	ActionBuffer  uint8 = 0x4
	ActionNotify  uint8 = 0x8
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

type ueSessionConfig struct {
	tc        uint8
	ueAddress string
	appFilter appFilter
	pdrs      []*ie.IE
	fars      []*ie.IE
	qers      []*ie.IE
}

type testCase struct {
	input    *pfcpSessionData
	expected ueSessionConfig

	desc string

	// modified by test cases only
	session *pfcpsim.PFCPSession
}

func init() {
	logger.SetLogLevel(zap.DebugLevel)
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
	ticker := time.NewTicker(500 * time.Millisecond)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker.C:
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
	ticker := time.NewTicker(500 * time.Millisecond)

	// Decrease timeout to wait for PFCP responses.
	// This decreases time to wait for PFCP Agent to start.
	pfcpClient.SetPFCPResponseTimeout(1 * time.Second)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker.C:
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

func setup(t *testing.T, configType uint32) {
	// TODO: we currently need to reset the DefaultRegisterer between tests, as some leave the
	// 		 the registry in a bad state. Use custom registries to avoid global state.
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	bessFake = fake_bess.NewFakeBESS()
	go func() {
		if err := bessFake.Run(":10514"); err != nil {
			panic(err)
		}
	}()
	err := waitForBESSFakeToStart()
	if err != nil {
		t.Fatalf("failed to start BESS fake: %v", err)
	}

	upfConf := GetConfig(configType)
	upfConf.N4Addr = "127.0.0.8"
	pfcpAgent = pfcpiface.NewPFCPIface(upfConf)
	go pfcpAgent.Run()

	pfcpClient = pfcpsim.NewPFCPClient("127.0.0.1")
	errConn := pfcpClient.ConnectN4("127.0.0.8")
	if errConn != nil {
		t.Fatalf("failed to connect to UPF: %v", errConn)
	}

	// wait for PFCP Agent to initialize, blocking
	err = waitForPFCPAssociationSetup(pfcpClient)
	if err != nil {
		t.Fatalf("failed to start PFCP Agent: %v", err)
	}
}

func teardown(t *testing.T) {
	if pfcpClient.IsAssociationAlive() {
		err := pfcpClient.TeardownAssociation()
		if err != nil {
			t.Errorf("Failed to teardown association: %v", err)
		}
	}

	if pfcpClient != nil {
		pfcpClient.DisconnectN4()
	}

	pfcpAgent.Stop()

	if bessFake != nil {
		bessFake.Stop()
	}
}

func verifyEntries(t *testing.T, expectedValues ueSessionConfig) {
	verifyBessEntries(t, bessFake, expectedValues)
}

func verifyNoEntries(t *testing.T) {
	verifyNoBessRuntimeEntries(t, bessFake)
}
