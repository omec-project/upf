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
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	simulate = simModeDisable
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

	pfcpIface.httpSrv = &http.Server{Addr: ":" + httpPort, Handler: nil}
	pfcpIface.upf = NewUPF(&conf, pfcpIface.fp)

	return pfcpIface
}

func (p *PFCPIface) Run() {
	if simulate.enable() {
		p.upf.sim(simulate, &p.conf.SimInfo)

		if !simulate.keepGoing() {
			return
		}
	}

	p.node = NewPFCPNode(p.upf)

	setupConfigHandler(p.upf)
	setupProm(p.upf, p.node)

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("http server failed", err)
		}

		log.Infoln("http server closed")
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	go func() {
		oscall := <-sig
		log.Infof("System call received: %+v", oscall)
		p.Stop()
	}()

	// blocking
	p.node.Serve()
}

// Stop sends cancellation signal to main Go routine and waits for shutdown to complete.
func (p *PFCPIface) Stop() {
	ctxHttpShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := p.httpSrv.Shutdown(ctxHttpShutdown); err != nil {
		log.Errorln("Failed to shutdown http: ", err)
	}

	p.node.Stop()

	// Wait for PFCP node shutdown
	p.node.Done()
}
