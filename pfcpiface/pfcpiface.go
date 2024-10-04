// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/omec-project/upf-epc/logger"
)

var (
	simulate = simModeDisable
)

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

	mu sync.Mutex
}

func NewPFCPIface(conf Conf) *PFCPIface {
	pfcpIface := &PFCPIface{
		conf: conf,
	}

	if conf.EnableP4rt {
		pfcpIface.fp = &UP4{}
	} else {
		pfcpIface.fp = &bess{}
	}

	httpPort := "8080"
	if conf.CPIface.HTTPPort != "" {
		httpPort = conf.CPIface.HTTPPort
	}

	pfcpIface.httpEndpoint = ":" + httpPort

	pfcpIface.upf = NewUPF(&conf, pfcpIface.fp)

	return pfcpIface
}

func (p *PFCPIface) mustInit() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.node = NewPFCPNode(p.upf)
	httpMux := http.NewServeMux()

	setupConfigHandler(httpMux, p.upf)

	var err error

	p.uc, p.nc, err = setupProm(httpMux, p.upf, p.node)

	if err != nil {
		logger.PfcpLog.Fatalln("setupProm failed", err)
	}

	// Note: due to error with golangci-lint ("Error: G112: Potential Slowloris Attack
	// because ReadHeaderTimeout is not configured in the http.Server (gosec)"),
	// the ReadHeaderTimeout is set to the same value as in nginx (client_header_timeout)
	p.httpSrv = &http.Server{Addr: p.httpEndpoint, Handler: httpMux, ReadHeaderTimeout: 60 * time.Second}
}

func (p *PFCPIface) Run() {
	if simulate.enable() {
		p.upf.sim(simulate, &p.conf.SimInfo)

		if !simulate.keepGoing() {
			return
		}
	}

	p.mustInit()

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.PfcpLog.Fatalln("http server failed", err)
		}

		logger.PfcpLog.Infoln("http server closed")
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	go func() {
		oscall := <-sig
		logger.PfcpLog.Infof("system call received: %+v", oscall)
		p.Stop()
	}()

	// blocking
	p.node.Serve()
}

// Stop sends cancellation signal to main Go routine and waits for shutdown to complete.
func (p *PFCPIface) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

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
}
