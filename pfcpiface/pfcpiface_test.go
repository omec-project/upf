// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The Linux Foundation

package pfcpiface

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// setUpfInfoDP is a fakeDP that records whether the agent set it up.
type setUpfInfoDP struct {
	fakeDP
	setUp bool
	err   error // returned by SetUpfInfo
}

func (d *setUpfInfoDP) SetUpfInfo(u *upf, conf *Conf) error {
	d.setUp = true

	return d.err
}

// loopbackName returns the name of this host's loopback interface: lo on
// Linux, lo0 on macOS.
func loopbackName(t *testing.T) string {
	t.Helper()

	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("listing interfaces: %v", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			return iface.Name
		}
	}

	t.Skip("no loopback interface")

	return ""
}

func TestNewPFCPIfaceUsesGivenDatapath(t *testing.T) {
	dp := &setUpfInfoDP{}

	p, err := newPFCPIface(loopbackConf(t), dp)
	if err != nil {
		t.Fatalf("newPFCPIface: %v", err)
	}

	if p.fp != dp {
		t.Errorf("agent datapath is %T, want the fake", p.fp)
	}

	if p.upf.datapath != dp {
		t.Errorf("upf datapath is %T, want the fake", p.upf.datapath)
	}

	if !dp.setUp {
		t.Error("SetUpfInfo was not called on the fake datapath")
	}
}

// loopbackConf returns a Conf that NewUPF accepts, with both interfaces on
// the loopback.
func loopbackConf(t *testing.T) Conf {
	t.Helper()

	lo := loopbackName(t)
	conf := Conf{RespTimeout: "2s"}
	conf.AccessIface.IfName = lo
	conf.CoreIface.IfName = lo

	return conf
}

// The agent used to exit, or keep a nil upf and crash later, on each of these.
func TestNewPFCPIfaceReturnsSetupErrors(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Conf)
		dp   *setUpfInfoDP
		want string
	}{
		{
			name: "access interface without an address",
			edit: func(c *Conf) { c.AccessIface.IfName = "no-such-interface" },
			want: "access interface",
		},
		{
			name: "bad resp_timeout",
			edit: func(c *Conf) { c.RespTimeout = "soon" },
			want: "resp_timeout",
		},
		{
			name: "bad urr_poll_interval",
			edit: func(c *Conf) {
				c.EnableUsageReporting = true
				c.URRPollInterval = "often"
			},
			want: "urr_poll_interval",
		},
		{
			name: "datapath setup fails",
			dp:   &setUpfInfoDP{err: errors.New("no BESS")},
			want: "no BESS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := loopbackConf(t)
			if tt.edit != nil {
				tt.edit(&conf)
			}

			dp := tt.dp
			if dp == nil {
				dp = &setUpfInfoDP{}
			}

			_, err := newPFCPIface(conf, dp)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("newPFCPIface error = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

// fakeResolver fails its first failures lookups, then returns hosts.
type fakeResolver struct {
	failures int
	hosts    []string
	calls    int
}

func (r *fakeResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	r.calls++
	if r.calls <= r.failures {
		return nil, errors.New("temporary DNS failure")
	}

	return r.hosts, nil
}

func withNodeIDResolver(t *testing.T, r hostResolver, tries int) {
	t.Helper()

	oldResolver, oldTries, oldPeriod := nodeIDResolver, nodeIDLookupTries, nodeIDLookupPeriod
	nodeIDResolver, nodeIDLookupTries, nodeIDLookupPeriod = r, tries, time.Millisecond

	t.Cleanup(func() {
		nodeIDResolver, nodeIDLookupTries, nodeIDLookupPeriod = oldResolver, oldTries, oldPeriod
	})
}

func TestResolveNodeIDRetriesThenSucceeds(t *testing.T) {
	r := &fakeResolver{failures: 2, hosts: []string{"192.0.2.1"}}
	withNodeIDResolver(t, r, 5)

	got, err := resolveNodeID(context.Background(), "upf.example")
	if err != nil {
		t.Fatalf("resolveNodeID: %v", err)
	}

	if got != "192.0.2.1" || r.calls != 3 {
		t.Errorf("got %q after %d lookups, want 192.0.2.1 after 3", got, r.calls)
	}
}

func TestResolveNodeIDGivesUp(t *testing.T) {
	r := &fakeResolver{failures: 100}
	withNodeIDResolver(t, r, 4)

	_, err := resolveNodeID(context.Background(), "upf.example")
	if err == nil || !strings.Contains(err.Error(), "temporary DNS failure") {
		t.Fatalf("resolveNodeID error = %v, want the last lookup error", err)
	}

	if r.calls != 4 {
		t.Errorf("%d lookups, want 4", r.calls)
	}
}

func TestResolveNodeIDNoAddresses(t *testing.T) {
	withNodeIDResolver(t, &fakeResolver{}, 2)

	_, err := resolveNodeID(context.Background(), "upf.example")
	if err == nil || !strings.Contains(err.Error(), "no addresses") {
		t.Fatalf("resolveNodeID error = %v, want no addresses", err)
	}
}

func TestResolveNodeIDStopsWhenCancelled(t *testing.T) {
	withNodeIDResolver(t, &fakeResolver{failures: 100}, 100)

	nodeIDLookupPeriod = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := resolveNodeID(ctx, "upf.example")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveNodeID error = %v, want context.Canceled", err)
	}
}

// useTestRegistry gives the agent's globally registered collectors a registry
// of their own for the rest of the test, not the default one the other tests
// use.
func useTestRegistry(t *testing.T) {
	t.Helper()

	oldRegisterer := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	t.Cleanup(func() { prometheus.DefaultRegisterer = oldRegisterer })
}

// freePort returns a TCP port nothing is listening on.
func freePort(t *testing.T) string {
	t.Helper()

	var lc net.ListenConfig

	free, err := lc.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("finding a port: %v", err)
	}
	defer free.Close()

	_, port, err := net.SplitHostPort(free.Addr().String())
	if err != nil {
		t.Fatalf("parsing %s: %v", free.Addr(), err)
	}

	return port
}

// runnableAgent returns an agent on dp that Run can start: PFCP on the
// loopback and HTTP on port.
func runnableAgent(t *testing.T, port string, dp datapath) *PFCPIface {
	t.Helper()

	conf := loopbackConf(t)
	conf.N4Addr = "127.0.0.1"
	conf.CPIface.HTTPPort = port

	p, err := newPFCPIface(conf, dp)
	if err != nil {
		t.Fatalf("newPFCPIface: %v", err)
	}

	return p
}

// runAgent starts Run and returns the channel its result arrives on.
func runAgent(p *PFCPIface) <-chan error {
	done := make(chan error, 1)

	go func() { done <- p.Run() }()

	return done
}

// waitForRun returns Run's result, failing the test if it takes too long.
func waitForRun(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return")

		return nil
	}
}

// waitForHTTP waits until the agent's HTTP server answers on port.
func waitForHTTP(t *testing.T, port string) {
	t.Helper()

	var d net.Dialer

	deadline := time.Now().Add(10 * time.Second)

	for {
		conn, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:"+port)
		if err == nil {
			conn.Close()

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("the agent's HTTP server didn't start: %v", err)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// exitDP is a fakeDP that records whether the agent released it.
type exitDP struct {
	fakeDP
	exited bool
}

func (d *exitDP) Exit() { d.exited = true }

// An HTTP server that can't start used to exit the agent from its goroutine.
// Now it stops the agent, and Run returns the error. The agent must also have
// released what it registered and set up, so another can start in its place.
func TestRunReturnsHTTPServerError(t *testing.T) {
	useTestRegistry(t)

	// Hold a port on every address, so the agent's HTTP server can't bind it.
	var lc net.ListenConfig

	busy, err := lc.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("holding a port: %v", err)
	}
	defer busy.Close()

	_, port, err := net.SplitHostPort(busy.Addr().String())
	if err != nil {
		t.Fatalf("parsing %s: %v", busy.Addr(), err)
	}

	dp := &exitDP{}

	err = waitForRun(t, runAgent(runnableAgent(t, port, dp)))
	if err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("Run returned %v, want the HTTP server's error", err)
	}

	if !dp.exited {
		t.Error("the agent didn't release its datapath")
	}

	// A replacement, in the same registry, starts and stops cleanly.
	port = freePort(t)
	p := runnableAgent(t, port, &fakeDP{})
	done := runAgent(p)

	waitForHTTP(t, port)
	p.Stop()

	if err := waitForRun(t, done); err != nil {
		t.Fatalf("the replacement agent's Run returned %v, want nil", err)
	}
}

// Run waits for the HTTP server's result, so check that Stop still ends it.
func TestRunReturnsNilAfterStop(t *testing.T) {
	useTestRegistry(t)

	port := freePort(t)
	p := runnableAgent(t, port, &fakeDP{})
	done := runAgent(p)

	// Stop needs the agent set up; wait until its HTTP server answers.
	waitForHTTP(t, port)
	p.Stop()

	if err := waitForRun(t, done); err != nil {
		t.Fatalf("Run returned %v after Stop, want nil", err)
	}
}

// When setup fails after NewUPF, Serve never runs to release the datapath, so
// Run has to.
func TestRunReleasesDatapathWhenSetupFails(t *testing.T) {
	useTestRegistry(t)

	conf := loopbackConf(t)
	conf.N4Addr = "not an address"

	dp := &exitDP{}

	p, err := newPFCPIface(conf, dp)
	if err != nil {
		t.Fatalf("newPFCPIface: %v", err)
	}

	if err := p.Run(); err == nil {
		t.Fatal("Run succeeded with a PFCP address it can't listen on")
	}

	if !dp.exited {
		t.Error("the agent didn't release its datapath")
	}
}

func TestFirstAddressIPWithoutAddresses(t *testing.T) {
	_, err := firstAddressIP("eth9", nil)
	if err == nil || !strings.Contains(err.Error(), "eth9 has no addresses") {
		t.Fatalf("firstAddressIP error = %v, want one saying eth9 has no addresses", err)
	}
}

// startAgent starts an agent on a free port and waits until it serves.
func startAgent(t *testing.T) (*PFCPIface, <-chan error) {
	t.Helper()

	port := freePort(t)
	p := runnableAgent(t, port, &fakeDP{})
	done := runAgent(p)

	waitForHTTP(t, port)

	return p, done
}

// Run used to return as soon as Stop had cancelled the node, while Stop was
// still unregistering the agent's collectors.
func TestRunWaitsForStopToFinish(t *testing.T) {
	useTestRegistry(t)

	p, done := startAgent(t)

	go p.Stop()

	if err := waitForRun(t, done); err != nil {
		t.Fatalf("Run returned %v after Stop, want nil", err)
	}

	if prometheus.Unregister(newUpfCollector(p.upf)) {
		t.Error("Run returned with the agent's UPF collector still registered")
	}
}

// Another Stop on a stopped agent used to unregister the metrics of the agent
// that replaced it, since Prometheus matches them by name.
func TestStopAgainLeavesReplacementMetrics(t *testing.T) {
	useTestRegistry(t)

	a, aDone := startAgent(t)
	a.Stop()

	if err := waitForRun(t, aDone); err != nil {
		t.Fatalf("A's Run returned %v, want nil", err)
	}

	b, bDone := startAgent(t)

	a.Stop()

	// B's PFCP metrics are still registered if a second service can't be.
	if s, err := metrics.NewPrometheusService(); err == nil {
		t.Error("stopping A again unregistered B's PFCP metrics")

		if err := s.Stop(); err != nil {
			t.Errorf("stopping the second service: %v", err)
		}
	}

	b.Stop()

	if err := waitForRun(t, bDone); err != nil {
		t.Fatalf("B's Run returned %v, want nil", err)
	}
}

// statsDP counts the session stats reads, each of which flips the BESS
// FlowMeasure buffers.
type statsDP struct {
	fakeDP
	reads atomic.Int32
}

func (d *statsDP) SessionStats(pc *PfcpNodeCollector, ch chan<- prometheus.Metric) error {
	d.reads.Add(1)
	return nil
}

func TestStopDoesNotReadSessionStats(t *testing.T) {
	useTestRegistry(t)

	port := freePort(t)
	dp := &statsDP{}

	p := runnableAgent(t, port, dp)
	p.upf.enableFlowMeasure = true

	done := runAgent(p)
	waitForHTTP(t, port)

	// Registering the node collector reads the stats once, which the PTF
	// linerate tests rely on until they flip the buffers themselves.
	before := dp.reads.Load()
	if before == 0 {
		t.Fatal("starting the agent didn't read the session stats")
	}

	p.Stop()

	if err := waitForRun(t, done); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}

	if after := dp.reads.Load(); after != before {
		t.Errorf("stopping the agent read the session stats %d times, want 0", after-before)
	}
}
