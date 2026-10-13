// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Intel Corporation

package pfcpiface

import (
	"context"
	"net"
	"time"

	"github.com/omec-project/upf-epc/logger"
)

const (
	// endMarkerRedialMin and endMarkerRedialMax bound the wait between attempts to
	// reach the datapath's End Marker socket. The first retry is quick because the
	// usual cause is an agent that started a moment before bessd created the socket.
	endMarkerRedialMin = 100 * time.Millisecond
	endMarkerRedialMax = 2 * time.Second
)

// dialFunc matches net.Dialer.DialContext, so tests can stand in for the datapath.
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// endMarkerLink owns the connection to the datapath's End Marker socket for the
// life of the process: it dials until the socket is there, forwards End Markers
// while connected, and dials again when the connection drops.
//
// End Markers it takes off the queue while it has no connection are discarded,
// so the PFCP side never blocks on a full queue; a marker delivered after the
// path switch it belongs to has completed is of no use to the gNB. Markers still
// queued when a dial succeeds are sent. A marker whose write fails is dropped,
// not retried on the next connection.
type endMarkerLink struct {
	addr      string
	dial      dialFunc
	markers   <-chan []byte
	stop      <-chan struct{}
	redialMin time.Duration
	redialMax time.Duration

	// wait is the delay before the next dial. It survives a dropped connection
	// and goes back to redialMin only once a connection has lasted that long.
	wait      time.Duration
	discarded int
}

// newEndMarkerLink returns a link to addr. A nil dial means a net.Dialer.
func newEndMarkerLink(addr string, dial dialFunc, markers <-chan []byte, stop <-chan struct{}) *endMarkerLink {
	if dial == nil {
		var d net.Dialer
		dial = d.DialContext
	}

	return &endMarkerLink{
		addr:      addr,
		dial:      dial,
		markers:   markers,
		stop:      stop,
		redialMin: endMarkerRedialMin,
		redialMax: endMarkerRedialMax,
	}
}

// run returns only when stop is closed.
func (l *endMarkerLink) run() {
	logger.BessLog.Infoln("end marker sender started, socket:", l.addr)
	defer logger.BessLog.Infoln("end marker sender stopped")

	l.wait = l.redialMin
	lost := false

	for {
		conn, ok := l.connect(lost)
		if !ok {
			return
		}

		connected := time.Now()
		ok = l.forward(conn)

		conn.Close()

		if !ok {
			return
		}

		// bessd serves one client at a time. Until it has seen the old
		// connection hang up it closes any new one as soon as it accepts it,
		// so redialing at once can produce a connection that is lost as soon
		// as it is made, again and again. Every redial after a drop waits, and
		// the wait keeps growing until a connection lasts.
		if time.Since(connected) >= l.redialMin {
			l.wait = l.redialMin
		}

		lost = true
	}
}

// connect dials until it succeeds or stop is closed. The first dial of the
// process is immediate; every other dial follows a wait.
func (l *endMarkerLink) connect(lost bool) (net.Conn, bool) {
	for attempt := 1; ; attempt++ {
		if lost || attempt > 1 {
			if !l.waitDiscarding(l.wait) {
				return nil, false
			}

			l.wait = min(2*l.wait, l.redialMax)
		}

		conn, err := l.dialOnce()
		if err == nil {
			if l.discarded > 0 {
				logger.BessLog.Warnf("discarded %d end marker(s) since the connection was lost", l.discarded)
				l.discarded = 0
			}

			logger.BessLog.Infof("connected to end marker socket %v after %d attempt(s)", l.addr, attempt)

			return conn, true
		}

		// One line per outage. The agent starting before bessd has created the
		// socket is expected, hence Warn; a lost connection has already been
		// logged by forward.
		if attempt == 1 && !lost {
			logger.BessLog.Warnf("end marker socket %v not reachable yet, retrying with backoff up to %v: %v", l.addr, l.redialMax, err)
		} else {
			logger.BessLog.Debugf("dial end marker socket %v failed (attempt %d): %v", l.addr, attempt, err)
		}
	}
}

// dialOnce makes one attempt that a closed stop channel abandons.
func (l *endMarkerLink) dialOnce() (net.Conn, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-l.stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	return l.dial(ctx, "unixpacket", l.addr)
}

// waitDiscarding waits for d, discarding End Markers, and reports false if stop
// was closed first.
func (l *endMarkerLink) waitDiscarding(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	for {
		select {
		case <-l.stop:
			return false
		case <-timer.C:
			return true
		case <-l.markers:
			l.discarded++
		}
	}
}

// forward writes End Markers to conn until the connection fails, and reports
// false if it returned because stop was closed.
//
// bessd only reads from this socket in the shipped configs
// (conf/closed_loop.bess:254), so a read ends only with an error. That error
// tells the sender the datapath went away without waiting for a marker write to
// fail.
func (l *endMarkerLink) forward(conn net.Conn) bool {
	dropped := make(chan error, 1)
	done := make(chan struct{})

	defer close(done)

	// A write to a datapath that has stopped reading blocks, and the select
	// below cannot see stop while it does. Closing the connection ends the
	// write.
	go func() {
		select {
		case <-l.stop:
			conn.Close()
		case <-done:
		}
	}()

	go func() {
		buf := make([]byte, 1)

		for {
			if _, err := conn.Read(buf); err != nil {
				dropped <- err
				return
			}
		}
	}()

	for {
		select {
		case <-l.stop:
			return false
		case err := <-dropped:
			// Stopping closes the connection too, so the read fails as stop is
			// closed and both cases can be ready at once.
			if l.stopping() {
				return false
			}

			logger.BessLog.Errorln("end marker socket connection lost, redialing:", err)

			return true
		case marker := <-l.markers:
			if _, err := conn.Write(marker); err != nil {
				if l.stopping() {
					return false
				}

				l.discarded++
				logger.BessLog.Errorln("end marker write failed, redialing:", err)

				return true
			}
		}
	}
}

func (l *endMarkerLink) stopping() bool {
	select {
	case <-l.stop:
		return true
	default:
		return false
	}
}
