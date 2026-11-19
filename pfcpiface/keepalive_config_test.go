// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/x2x3"
)

// TestX2X3KeepaliveConfigDefaultsWhenUnusable.
//
// An unusable timer is not refused by validateConf, because its error reaches
// logger.InitLog.Fatalln in cmd/pfcpiface/main.go: the UPF would exit over a keepalive
// timer, taking the user plane with it, and print the LI field name into the general
// operator log on the way out. Two things an LI value may not do.
//
// The specification supplies both defaults normatively (60s and 180s), so there is
// something correct to fall back to — which is what the AMF and SMF do with the same
// two settings. An unusable value is reported to the ADMF and the defaults are used.
//
// The pair worth the test is TIME_P2 below TIME_P1: both parse, both look reasonable in
// a file, and together they disconnect every X3 connection before the keepalive that
// would keep it is sent — so falling back to the defaults has to discard *both*, not
// keep the half that parsed.
func TestX2X3KeepaliveConfigDefaultsWhenUnusable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		p1, p2     string
		wantReport bool
		wantP1     time.Duration
		wantP2     time.Duration
	}{
		{name: "nothing configured takes the specification's defaults"},
		{name: "a usable pair", p1: "10s", p2: testKeepaliveP1, wantP1: 10 * time.Second, wantP2: 30 * time.Second},
		{name: "only TIME_P1, with TIME_P2 defaulting above it", p1: testKeepaliveP1, wantP1: 30 * time.Second},
		{name: "an unparseable TIME_P1", p1: "sixty", wantReport: true},
		{name: "an unparseable TIME_P2", p2: "three minutes", wantReport: true},
		{name: "TIME_P2 equal to TIME_P1", p1: testKeepaliveP2, p2: testKeepaliveP2, wantReport: true},
		{name: "TIME_P2 below TIME_P1", p1: testKeepaliveP2, p2: testKeepaliveP1, wantReport: true},
		// Only TIME_P2, set below the default TIME_P1 of 60s: the file reads as though
		// one timer was tightened, and the mechanism becomes a disconnect loop.
		{name: "only TIME_P2, below the default TIME_P1", p2: testKeepaliveP1, wantReport: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			li := &LiConfig{
				X3SockAddr: "/tmp/x3.sock", Cert: "c", Key: "k", CACert: "ca",
				X1Listen: testX1ListenAddr, TFID: testTFID, NEID: testNEID,
				X2X3KeepaliveTimeP1: tc.p1, X2X3KeepaliveTimeP2: tc.p2,
			}

			// Whatever the timers say, the configuration must not stop the process:
			// validateConf does not look at the li block at all.
			if err := validateConf(Conf{Mode: testModeSim, RespTimeout: "2s", ReadTimeout: 15, MaxReqRetries: 5, Li: li}); err != nil {
				t.Errorf("validateConf() error = %v; an LI timer must not stop the user plane", err)
			}

			reporter := &recordingReporter{}
			got := keepaliveConfig(*li, reporter)

			if got := reporter.reported(); (len(got) > 0) != tc.wantReport {
				t.Errorf("reported = %v, want %v (%v)", len(got) > 0, tc.wantReport, got)
			}
			// Zero means "the specification's own value", which x2x3 resolves.
			if got.TimeP1 != tc.wantP1 || got.TimeP2 != tc.wantP2 {
				t.Errorf("timers = (%s, %s), want (%s, %s)", got.TimeP1, got.TimeP2, tc.wantP1, tc.wantP2)
			}
		})
	}
}

// TestX2X3KeepaliveConfigCarriesTheOperatorsIntent: what config.go accepts must be what
// the shipper runs, including the tri-state switch.
func TestX2X3KeepaliveConfigCarriesTheOperatorsIntent(t *testing.T) {
	enabled, disabled := true, false

	for _, tc := range []struct {
		name         string
		cfg          LiConfig
		wantDisabled bool
		wantP1       time.Duration
	}{
		{name: "unset runs the mechanism"},
		{name: "explicitly enabled", cfg: LiConfig{X2X3KeepaliveEnabled: &enabled}},
		{name: "explicitly disabled", cfg: LiConfig{X2X3KeepaliveEnabled: &disabled}, wantDisabled: true},
		{name: "a configured timer", cfg: LiConfig{X2X3KeepaliveTimeP1: "5s"}, wantP1: 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := keepaliveConfig(tc.cfg, nil)
			if got.Disabled != tc.wantDisabled {
				t.Errorf("Disabled = %v, want %v", got.Disabled, tc.wantDisabled)
			}
			if got.TimeP1 != tc.wantP1 {
				t.Errorf("TimeP1 = %s, want %s", got.TimeP1, tc.wantP1)
			}
		})
	}
}

// TestX2X3KeepaliveReachesTheShippersClients closes the gap between "the settings were
// parsed" and "the settings are what the connection runs".
//
// This POI is the one that does not use x2x3.Pool — it builds its clients in senderFor —
// so nothing else covers the path from configuration to connection. Without this, senderFor
// could be changed to pass a zero KeepaliveConfig and every other test would still pass
// while the UPF silently stopped keepaliving X3.
//
// Asserted through the wire rather than through the struct: what matters is that keepalives
// arrive at the mediation function at the configured period, not that a field was copied.
func TestX2X3KeepaliveReachesTheShippersClients(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	mdfCert, mdfKey := liLeaf(t, dir, caCert, caKey, "MDF", "mdf3-1")
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)

	mdfMat, err := mtls.Load(mdfCert, mdfKey, caPath)
	if err != nil {
		t.Fatalf("load mdf material: %v", err)
	}
	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	// A mediation function that does nothing but count what it is sent.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", mdfMat.ServerTLS())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var (
		mu         sync.Mutex
		keepalives int
	)
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					var head [12]byte
					if _, readErr := io.ReadFull(c, head[:]); readErr != nil {
						return
					}
					headerLen := binary.BigEndian.Uint32(head[4:8])
					payloadLen := binary.BigEndian.Uint32(head[8:12])
					rest := make([]byte, int(headerLen)+int(payloadLen)-len(head))
					if _, readErr := io.ReadFull(c, rest); readErr != nil {
						return
					}
					if binary.BigEndian.Uint16(head[2:4]) == uint16(x2x3.PDUTypeKeepalive) {
						mu.Lock()
						keepalives++
						mu.Unlock()
					}
				}
			}(conn)
		}
	}()

	// The shipper as senderFor needs it: credentials, the operator's keepalive settings,
	// and somewhere to keep the clients it builds. TIME_P2 is long because this test is
	// about the send, and this mediation function acknowledges nothing.
	// The mechanism is driven directly rather than through keepaliveConfig, because
	// what this test needs is a timer fast enough to observe and "25ms" is not a value
	// an operator may write: TS 103 221-2 clause 6.2.4 expresses these in integer
	// seconds, so KeepaliveConfig.Validate — which keepaliveConfig applies to the
	// operator's input — refuses anything under a second and falls back to the
	// specification's own timers. Going through that path would test the validation,
	// not the sending, and would wait sixty seconds to do it.
	s := &liShipper{
		tlsConfig: upfMat.ClientTLS(),
		keepalive: x2x3.KeepaliveConfig{TimeP1: 25 * time.Millisecond, TimeP2: time.Hour},
		senders:   make(map[string]x2x3.Sender),
	}

	sender, err := s.senderFor(ln.Addr().String())
	if err != nil {
		t.Fatalf("senderFor: %v", err)
	}
	defer sender.Close()

	// A keepalive never dials, so an xCC is what opens the connection it runs on.
	if err := sender.Send(&x2x3.PDU{
		Type:          x2x3.PDUTypeX3,
		PayloadFormat: x2x3.PayloadFormatIPv4,
		Direction:     x2x3.DirectionToTarget,
		Payload:       []byte{0x45, 0x00},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := keepalives
		mu.Unlock()

		if n >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	n := keepalives
	mu.Unlock()
	t.Errorf("the mediation function received %d keepalives in 5s at TIME_P1=25ms; "+
		"the shipper's clients are not running the configured mechanism", n)
}

// TestAnUnusableLIBlockDoesNotStopTheUserPlane: a mistake in the `li` block must not stop the
// user plane.
//
// validateConf's error reaches logger.InitLog.Fatalln in cmd/pfcpiface/main.go, so if it
// refused an unusable `li` field a mistyped optional LI value would exit the process,
// taking the **user plane** with it, after printing the field name and value into the
// general operator log. An LI typo causing a network outage, and an LI-attributable line in
// a log far more widely readable than the config file it describes. bess.go's own handling
// of a shipper failure is deliberately vague for exactly that reason.
//
// Loading must succeed for every one of these, and the shipper must refuse them.
func TestAnUnusableLIBlockDoesNotStopTheUserPlane(t *testing.T) {
	complete := func() *LiConfig {
		return &LiConfig{
			X3SockAddr: "/pod-share/x3", Cert: "c", Key: "k", CACert: "ca",
			X1Listen: testX1ListenAddr, TFID: testTFID, NEID: testNEID,
		}
	}

	for _, tc := range []struct {
		name  string
		spoil func(*LiConfig)
	}{
		{"no ne_id", func(c *LiConfig) { c.NEID = "" }},
		{"no tf_id", func(c *LiConfig) { c.TFID = "" }},
		{"no x1_listen", func(c *LiConfig) { c.X1Listen = "" }},
		{"no x3_sockaddr", func(c *LiConfig) { c.X3SockAddr = "" }},
		{"no credentials", func(c *LiConfig) { c.Cert = "" }},
		{"an unparseable keepalive_timeout", func(c *LiConfig) { c.KeepaliveTimeout = "5min" }},
		{"a keepalive_timeout below the floor", func(c *LiConfig) { c.KeepaliveTimeout = testKeepaliveP1 }},
		{"an unparseable trigger_keepalive", func(c *LiConfig) { c.TriggerKeepalive = "5min" }},
		{"a trigger_keepalive below the floor", func(c *LiConfig) { c.TriggerKeepalive = testKeepaliveP1 }},
		{"keepalive_timeout and trigger_keepalive differing", func(c *LiConfig) {
			c.KeepaliveTimeout, c.TriggerKeepalive = "5m", "10m"
		}},
		{"an unparseable keepalive_time_p1", func(c *LiConfig) { c.KeepaliveTimeP1 = "1min" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			li := complete()
			tc.spoil(li)

			conf := Conf{Mode: testModeSim, RespTimeout: "2s", ReadTimeout: 15, MaxReqRetries: 5, Li: li}
			if err := validateConf(conf); err != nil {
				t.Errorf("validateConf refused the configuration (%v). That reaches Fatalln "+
					"in main and stops the user plane over a Lawful Interception value.", err)
			}
			if err := validateLiConfig(li); err == nil {
				t.Error("the shipper accepted a configuration it cannot carry out; interception " +
					"would look enabled and produce nothing")
			}
		})
	}
}

// TestAnUnusableLIBlockNamesNoFieldInTheGeneralLog is the disclosure half. The refusal
// travels as an error to startLIShipper's caller, which logs it non-attributably; what
// must not happen is the field name reaching a general sink, as it would through Fatalln.
func TestAnUnusableLIBlockNamesNoFieldInTheGeneralLog(t *testing.T) {
	var logged bytes.Buffer

	restore := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(restore) })

	li := &LiConfig{
		X3SockAddr: "/pod-share/x3", Cert: "c", Key: "k", CACert: "ca",
		X1Listen: testX1ListenAddr, TFID: testTFID, // ne_id missing
	}

	if err := validateConf(Conf{Mode: testModeSim, RespTimeout: "2s", ReadTimeout: 15, MaxReqRetries: 5, Li: li}); err != nil {
		t.Fatalf("validateConf refused the configuration: %v", err)
	}
	if err := validateLiConfig(li); err == nil {
		t.Fatal("the shipper accepted an li block with no ne_id")
	}

	if strings.Contains(logged.String(), "li.") || strings.Contains(logged.String(), "ne_id") {
		t.Errorf("an LI field name reached the general log:\n%s", logged.String())
	}
}
