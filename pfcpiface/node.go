package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	"github.com/pion/dtls/v2"
	"github.com/pion/dtls/v2/examples/util"
	"github.com/pion/dtls/v2/pkg/crypto/selfsign"
	"github.com/pion/udp"
	log "github.com/sirupsen/logrus"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

// PFCPNode represents a PFCP endpoint of the UPF.
type PFCPNode struct {
	ctx context.Context
	// listening socket for new "PFCP connections"
	net.Listener
	// done is closed to signal shutdown complete
	done chan struct{}
	// channel for PFCPConn to signal exit by sending their remote address
	pConnDone chan string
	// map of existing connections
	pConns map[string]*PFCPConn
	// upf
	upf *upf
	// metrics for PFCP messages and sessions
	metrics metrics.InstrumentPFCP
}

// NewPFCPNode create a new PFCPNode listening on local address.
func NewPFCPNode(ctx context.Context, tlsEnabled bool, upf *upf) *PFCPNode {
	addr, err := net.ResolveUDPAddr("udp", ":"+PFCPPort)
	if err != nil {
		log.Fatalln("Resolve local address failed", err)
	}

	listener, err := udp.Listen(addr.Network(), addr)
	if err != nil {
		log.Fatalln("ListenUDP failed", err)
	}

	if tlsEnabled {
		certificate, genErr := selfsign.GenerateSelfSigned()
		util.Check(genErr)
		// Prepare the configuration of the DTLS connection
		config := &dtls.Config{
			Certificates:         []tls.Certificate{certificate},
			InsecureSkipVerify:   true,
			ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
		}

		listener, err = dtls.NewListener(listener, config)
		if err != nil {
			log.Fatalln("Unable to create dTLS listener", err)
		}
	}

	metrics, err := metrics.NewPrometheusService()
	if err != nil {
		log.Fatalln("prom metrics service init failed", err)
	}

	return &PFCPNode{
		ctx:       ctx,
		Listener:  listener,
		done:      make(chan struct{}),
		pConnDone: make(chan string, 100),
		pConns:    make(map[string]*PFCPConn),
		upf:       upf,
		metrics:   metrics,
	}
}

func (node *PFCPNode) handleNewPeers() {
	lAddrStr := node.Addr().String()
	log.Infoln("listening for new PFCP connections on", lAddrStr)

	for {

		conn, err := node.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}

		node.NewPFCPConn(conn)
	}
}

// Serve listens for the first packet from a new PFCP peer and creates PFCPConn.
func (node *PFCPNode) Serve() {
	go node.handleNewPeers()

	shutdown := false

	for !shutdown {
		select {
		case fseid := <-node.upf.reportNotifyChan:
			// TODO: Logic to distinguish PFCPConn based on SEID
			for _, pConn := range node.pConns {
				pConn.handleDigestReport(fseid)
				break
			}
		case rAddr := <-node.pConnDone:
			delete(node.pConns, rAddr)
			log.Infoln("Removed connection to", rAddr)
		case <-node.ctx.Done():
			shutdown = true
			log.Infoln("Entering node shutdown")

			err := node.Close()
			if err != nil {
				log.Errorln("Error closing PFCPNode Conn", err)
			}

			// Clear out the remaining pconn completions
			for len(node.pConns) > 0 {
				rAddr := <-node.pConnDone
				delete(node.pConns, rAddr)
				log.Infoln("Removed connection to", rAddr)
			}

			close(node.pConnDone)
			log.Infoln("Done waiting for PFCPConn completions")
		}
	}

	close(node.done)
}

// Done waits for Shutdown() to complete
func (node *PFCPNode) Done() {
	<-node.done
	log.Infoln("Shutdown complete")
	return
}
