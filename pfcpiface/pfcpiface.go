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

	fp  fastPath
	upf *upf
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

	httpSrv := &http.Server{Addr: ":" + httpPort, Handler: nil}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("http server failed", err)
		}

		log.Infoln("http server closed")
	}()

	ctx, cancel := context.WithCancel(context.Background())

	node := NewPFCPNode(ctx, p.upf)
	go node.Serve()

	setupProm(p.upf, node)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

	cancel()

	// Wait for node shutdown before http shutdown
	node.Done()

	if err := httpSrv.Shutdown(context.Background()); err != nil {
		log.Errorln("Failed to shutdown http:", err)
	}

	p.upf.exit()
}
