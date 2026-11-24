// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The Linux Foundation

package pfcpiface

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

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

// runnableAgent returns an agent on a fake datapath that Run can start: PFCP
// on the loopback and HTTP on port. Its collectors go in a registry of their
// own, not the default one the other tests use.
func runnableAgent(t *testing.T, port string) *PFCPIface {
	t.Helper()

	oldRegisterer := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	t.Cleanup(func() { prometheus.DefaultRegisterer = oldRegisterer })

	conf := loopbackConf(t)
	conf.N4Addr = "127.0.0.1"
	conf.CPIface.HTTPPort = port

	p, err := newPFCPIface(conf, &fakeDP{})
	if err != nil {
		t.Fatalf("newPFCPIface: %v", err)
	}

	return p
}

// An HTTP server that can't start used to exit the agent from its goroutine.
// Now it stops the agent, and Run returns the error.
func TestRunReturnsHTTPServerError(t *testing.T) {
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

	p := runnableAgent(t, port)

	done := make(chan error, 1)

	go func() { done <- p.Run() }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "http server") {
			t.Fatalf("Run returned %v, want the HTTP server's error", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after its HTTP server failed")
	}
}

// Run waits for the HTTP server's result, so check that Stop still ends it.
func TestRunReturnsNilAfterStop(t *testing.T) {
	// Find a free port for the HTTP server.
	var lc net.ListenConfig

	free, err := lc.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("finding a port: %v", err)
	}

	_, port, err := net.SplitHostPort(free.Addr().String())
	if err != nil {
		t.Fatalf("parsing %s: %v", free.Addr(), err)
	}

	free.Close()

	p := runnableAgent(t, port)

	done := make(chan error, 1)

	go func() { done <- p.Run() }()

	// Stop needs the agent set up; wait until its HTTP server answers.
	var d net.Dialer

	deadline := time.Now().Add(10 * time.Second)

	for {
		conn, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:"+port)
		if err == nil {
			conn.Close()

			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("the agent's HTTP server didn't start: %v", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	p.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after Stop, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}
