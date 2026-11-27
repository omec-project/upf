// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/omec-project/upf-epc/logger"
	"github.com/prometheus/client_golang/prometheus"
)

var simulate = simModeDisable

func init() {
	flag.Var(&simulate, "simulate", "create|delete|create_continue simulated sessions")
}

type PFCPIface struct {
	conf Conf

	node *PFCPNode
	fp   datapath
	upf  *upf

	httpSrv      *http.Server
	httpEndpoint string

	uc *upfCollector
	nc *PfcpNodeCollector

	mu      sync.Mutex
	stopped bool // Stop has shut the agent down; protected by mu

	// stopDone is closed when Stop has finished. Set up by setUp.
	stopDone chan struct{}
}

// NewPFCPIface returns a PFCP agent that programs BESS.
func NewPFCPIface(conf Conf) (*PFCPIface, error) {
	return newPFCPIface(conf, &bess{})
}

// newPFCPIface returns a PFCP agent that programs fp, so tests can run the
// agent against a fake datapath.
func newPFCPIface(conf Conf, fp datapath) (*PFCPIface, error) {
	pfcpIface := &PFCPIface{
		conf: conf,
		fp:   fp,
	}

	httpPort := "8080"
	if conf.CPIface.HTTPPort != "" {
		httpPort = conf.CPIface.HTTPPort
	}

	pfcpIface.httpEndpoint = ":" + httpPort

	upf, err := NewUPF(&conf, pfcpIface.fp)
	if err != nil {
		return nil, err
	}

	pfcpIface.upf = upf

	return pfcpIface, nil
}

func (p *PFCPIface) setUp() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var err error

	p.node, err = NewPFCPNode(p.upf)
	if err != nil {
		return err
	}

	httpMux := http.NewServeMux()

	setupConfigHandler(httpMux, p.upf)

	p.uc, p.nc, err = setupProm(httpMux, p.upf, p.node)
	if err != nil {
		// Serve hasn't started, so stop the node, which unregisters its
		// metrics, without waiting for Serve to finish.
		p.node.Stop()
		p.node.Close()

		return fmt.Errorf("setting up Prometheus metrics: %w", err)
	}

	// Note: due to error with golangci-lint ("Error: G112: Potential Slowloris Attack
	// because ReadHeaderTimeout is not configured in the http.Server (gosec)"),
	// the ReadHeaderTimeout is set to the same value as in nginx (client_header_timeout)
	p.httpSrv = &http.Server{Addr: p.httpEndpoint, Handler: httpMux, ReadHeaderTimeout: 60 * time.Second}
	p.stopDone = make(chan struct{})

	return nil
}

// Run serves PFCP until the agent is stopped, by a signal or by Stop. It
// returns an error if the agent couldn't start, or if its HTTP server failed,
// which stops the agent.
func (p *PFCPIface) Run() error {
	if simulate.enable() {
		p.upf.sim(simulate, &p.conf.SimInfo)

		if !simulate.keepGoing() {
			return nil
		}
	}

	if err := p.setUp(); err != nil {
		// Serve releases the datapath when it stops; it won't run, so release
		// the datapath NewUPF set up here.
		p.upf.Exit()

		return err
	}

	// The HTTP server's result. Every way Serve returns, through Stop, also ends
	// the server: Stop shuts it down, or it failed and called Stop itself. So
	// this always gets a value, sent after any Stop the failure called has
	// finished.
	httpErr := make(chan error, 1)

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.PfcpLog.Errorln("http server failed", err)
			p.Stop()

			httpErr <- fmt.Errorf("http server: %w", err)

			return
		}

		logger.PfcpLog.Infoln("http server closed")

		httpErr <- nil
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	// Run can return without a signal, so stop listening for one when it
	// does, rather than leave the subscription and this goroutine behind.
	defer signal.Stop(sig)

	go func() {
		select {
		case oscall := <-sig:
			logger.PfcpLog.Infof("system call received: %+v", oscall)
			p.Stop()
		case <-p.stopDone:
		}
	}()

	// blocking
	p.node.Serve()

	err := <-httpErr

	// Serve returns once Stop has cancelled the node, and the server once Stop
	// has shut it down, both before Stop has finished cleaning up. Wait for
	// it, so a caller can start another agent as soon as Run returns.
	<-p.stopDone

	return err
}

// Stop sends cancellation signal to main Go routine and waits for shutdown to complete.
func (p *PFCPIface) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop runs once. A second call, from a signal and from a failed HTTP
	// server, or by a caller after starting a replacement, would unregister
	// the replacement's metrics: Prometheus matches them by name, not by
	// instance. Nothing to stop unless Run set the agent up.
	if p.stopped || p.stopDone == nil {
		return
	}

	p.stopped = true
	defer close(p.stopDone)

	ctxHttpShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := p.httpSrv.Shutdown(ctxHttpShutdown); err != nil {
		logger.PfcpLog.Errorln("failed to shutdown http:", err)
	}

	p.node.Stop()

	// Wait for PFCP node shutdown
	p.node.Done()

	// Unregister the UPF collector, so another agent in this process can start.
	// The PFCP node collector stays registered: it describes itself by
	// collecting, so unregistering it would read the session stats, flipping
	// the FlowMeasure buffers at shutdown, and Prometheus can't unregister it
	// anyway when that read returns nothing. It registers again without
	// conflict for the same reason.
	if !prometheus.Unregister(p.uc) {
		logger.PfcpLog.Warnln("failed to unregister upfCollector")
	}

	// The datapath write histogram is registered with the UPF collector and goes
	// with it.
	if !prometheus.Unregister(datapathWriteDuration) {
		logger.PfcpLog.Warnln("failed to unregister datapathWriteDuration")
	}
}
