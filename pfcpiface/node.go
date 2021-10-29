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
	// done is closed to signal shutdown complete
	done chan struct{}
	// channel for PFCPConn to signal exit by sending their remote address
	pConnDone chan string
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
		done:       make(chan struct{}),
		pConnDone:  make(chan string, 100),
		pconns:     make(map[string]*PFCPConn),
		upf:        upf,
	}
}

func (node *PFCPNode) serve() {
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
			log.Warnln("Drop packet for existing PFCPconn received from", rAddrStr)
			continue
		}

		log.Infoln(lAddrStr, "received new connection from", rAddrStr)

		p := NewPFCPConn(node.ctx, node.upf, node.pConnDone, lAddrStr, rAddrStr)
		node.pconns[rAddrStr] = p
		p.HandlePFCPMsg(buf[:n])

		go p.Serve()
	}
}

func (node *PFCPNode) waitPFCPConnCompletions() {
	for {
		select {
		case rAddr := <-node.pConnDone:
			log.Infoln("Removing connection to", rAddr)
			delete(node.pconns, rAddr)
		case <-node.ctx.Done():
			log.Infoln("Stop waiting for PFCPConn completions")
			return
		}
	}
}

// Serve listens for the first packet from a new PFCP peer and creates PFCPConn.
func (node *PFCPNode) Serve() {
	log.Infoln("listening for new PFCP connections on", node.LocalAddr().String())

	go node.serve()

	go node.waitPFCPConnCompletions()

	<-node.ctx.Done()
	node.Shutdown()
}

// Shutdown closes it's connection and issues delete all sessions to fastpath
func (node *PFCPNode) Shutdown() {
	err := node.Close()
	if err != nil {
		log.Errorln("Error closing Conn", err)
	}

	node.upf.sendDeleteAllSessionsMsgtoUPF()
	close(node.done)
}

// Done waits for Shutdown() to complete
func (node *PFCPNode) Done() {
	<-node.done
	log.Infoln("PFCPNode: Shutdown complete")
	return
}
