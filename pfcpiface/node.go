package main

import (
	"context"
	"errors"
	"net"

	reuse "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"
)

// PFCPNode represents a PFCP endpoint of the UPF.
type PFCPNode struct {
	ctx context.Context
	// listening socket for new "PFCP connections"
	net.PacketConn
	// channel for PFCPConn to signal exit by sending their remote address
	done chan string
	// map of existing connections
	pconns map[string]*PFCPConn
	// upf
	upf *upf
}

// NewPFCPNode create a new PFCPNode listening on local address.
func NewPFCPNode(ctx context.Context, upf *upf) *PFCPNode {
	lAddr := upf.n4SrcIP.String() + ":" + PFCPPort

	conn, err := reuse.ListenPacket("udp", lAddr)
	if err != nil {
		log.Fatalln("ListenUDP failed", err)
	}

	return &PFCPNode{
		ctx:        ctx,
		PacketConn: conn,
		done:       make(chan string, 100),
		pconns:     make(map[string]*PFCPConn),
		upf:        upf,
	}
}

// Serve listens for the first packet from a new PFCP peer and creates PFCPConn.
func (node *PFCPNode) Serve() {
	log.Infoln("listening for new PFCP connections on", node.LocalAddr().String())

	go func() {
		lAddrStr := node.LocalAddr().String()

		for {
			buf := make([]byte, 1024)

			n, rAddr, err := node.ReadFrom(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				continue
			}

			rAddrStr := rAddr.String()

			_, ok := node.pconns[rAddrStr]
			if ok {
				log.Warnln("Dropping packet for existing PFCPconn received from", rAddrStr)
				continue
			}

			log.Infoln(lAddrStr, "received new connection from", rAddrStr)

			p := NewPFCPConn(node.ctx, node.upf, node.done, lAddrStr, rAddrStr)
			node.pconns[rAddrStr] = p
			p.HandlePFCPMsg(buf[:n])

			go p.Serve()
		}
	}()

	go func(ctx context.Context) {
		for {
			select {
			case rAddr := <-node.done:
				delete(node.pconns, rAddr)
			case <-ctx.Done():
				return
			}
		}
	}(node.ctx)

	<-node.ctx.Done()
	node.Shutdown()
}

// Shutdown closes it's connection and issues shutdown to all PFCPConn.
func (node *PFCPNode) Shutdown() {
	err := node.Close()
	if err != nil {
		log.Errorln("Error closing Conn", err)
	}

	log.Infoln("PFCPNode: Shutdown complete")
}
