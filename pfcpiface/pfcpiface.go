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

	log "github.com/sirupsen/logrus"
)

var (
	simulate = simModeDisable
	pfcpsim  = flag.Bool("pfcpsim", false, "simulate PFCP")
)

func init() {
	flag.Var(&simulate, "simulate", "create|delete|create_continue simulated sessions")
}

type PFCPIface struct {
	conf Conf

	node    *PFCPNode
	fp      fastPath
	upf     *upf
	httpSrv *http.Server

	stop    context.CancelFunc
	wg      *sync.WaitGroup
}

func NewPFCPIface(conf Conf) *PFCPIface {
	pfcpIface := &PFCPIface{
		conf: conf,
		wg:   &sync.WaitGroup{},
	}

	if conf.EnableP4rt {
		pfcpIface.fp = &UP4{}
	} else {
		pfcpIface.fp = &bess{}
	}

	pfcpIface.upf = NewUPF(&conf, pfcpIface.fp)

	return pfcpIface
}

func (p *PFCPIface) Run() {
	if *pfcpsim {
		pfcpSim()
		return
	}

	if simulate.enable() {
		p.upf.sim(simulate, &p.conf.SimInfo)

		if !simulate.keepGoing() {
			return
		}
	}

	setupConfigHandler(p.upf)

	httpPort := "8080"
	if p.conf.CPIface.HTTPPort != "" {
		httpPort = p.conf.CPIface.HTTPPort
	}

	p.httpSrv = &http.Server{Addr: ":" + httpPort, Handler: nil}

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("http server failed", err)
		}

		log.Infoln("http server closed")
	}()

	ctx, cancel := context.WithCancel(context.Background())
	p.stop = cancel

	p.node = NewPFCPNode(ctx, p.upf)
	go p.node.Serve()

	setupProm(p.upf, p.node)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	go func() {
		oscall := <-sig
		log.Infof("System call received: %+v", oscall)
		p.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			p.shutdown()
			return
		}
	}
}

func (p *PFCPIface) shutdown() {
	defer p.wg.Done()

	// Wait for node shutdown before http shutdown
	p.node.Done()

	if err := p.httpSrv.Shutdown(context.Background()); err != nil {
		log.Errorln("Failed to shutdown http: ", err)
	}

	p.upf.exit()
}

// Stop sends cancellation signal to main Go routine and waits for shutdown to complete.
func (p *PFCPIface) Stop() {
	p.wg.Add(1)
	p.stop()
	p.wg.Wait()
}
