// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Intel Corporation

package pfcpiface

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

var errSocketMissing = errors.New("connect: no such file or directory")

// setUpfInfoWithoutDatapath runs SetUpfInfo against a BESS address nothing
// listens on, so its datapath calls fail fast, with GTP-U path monitoring
// cleared first so the test can see whether SetUpfInfo set it. Exit runs when
// the test ends and waits for the End Marker sender; the globals are put back
// after that.
//
// The tests that use it write the globals *bessIP and enableGtpuPathMonitoring,
// so they must not run in parallel.
func setUpfInfoWithoutDatapath(t *testing.T, b *bess, conf *Conf) {
	t.Helper()

	savedBessIP := *bessIP
	savedPathMonitoring := enableGtpuPathMonitoring

	t.Cleanup(func() {
		*bessIP = savedBessIP
		enableGtpuPathMonitoring = savedPathMonitoring
	})

	*bessIP = "127.0.0.1:1"
	enableGtpuPathMonitoring = false

	b.SetUpfInfo(&upf{}, conf)

	t.Cleanup(b.Exit)
}

// scriptedDatapath stands in for the datapath's End Marker socket. Each dial
// consumes the next outcome: an error, or a connection whose datapath end is
// handed to the test.
type scriptedDatapath struct {
	mu       sync.Mutex
	failures int
	dials    int
	accepted chan net.Conn
}

func newScriptedDatapath(failures int) *scriptedDatapath {
	return &scriptedDatapath{failures: failures, accepted: make(chan net.Conn, 4)}
}

func (s *scriptedDatapath) dial(ctx context.Context, network, address string) (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dials++
	if s.dials <= s.failures {
		return nil, errSocketMissing
	}

	agent, datapath := net.Pipe()
	s.accepted <- datapath

	return agent, nil
}

func (s *scriptedDatapath) dialCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.dials
}

func (s *scriptedDatapath) accept(t *testing.T, what string) net.Conn {
	t.Helper()

	select {
	case conn := <-s.accepted:
		t.Cleanup(func() { conn.Close() })
		return conn
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: the link never connected", what)
		return nil
	}
}

// startTestLink runs a link with short redial waits, which tune may change, and
// stops it when the test ends. The function it returns stops the link and
// returns a channel closed once run has returned.
func startTestLink(t *testing.T, dial dialFunc, tune ...func(*endMarkerLink)) (chan<- []byte, func() <-chan struct{}) {
	t.Helper()

	markers := make(chan []byte, 16)
	stop := make(chan struct{})

	l := newEndMarkerLink("/test/pfcpport", dial, markers, stop)
	l.redialMin = time.Millisecond
	l.redialMax = 5 * time.Millisecond

	for _, f := range tune {
		f(l)
	}

	done := make(chan struct{})

	go func() {
		l.run()
		close(done)
	}()

	var once sync.Once

	stopLink := func() <-chan struct{} {
		once.Do(func() { close(stop) })
		return done
	}

	// A link that cannot stop fails the test here rather than hanging it.
	t.Cleanup(func() {
		select {
		case <-stopLink():
		case <-time.After(5 * time.Second):
			t.Error("the link was still running when the test ended")
		}
	})

	return markers, stopLink
}

func expectEndMarker(t *testing.T, markers chan<- []byte, conn net.Conn, want []byte, what string) {
	t.Helper()

	markers <- want

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	buf := make([]byte, 64)

	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("%s: no End Marker reached the datapath: %v", what, err)
	}

	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("%s: got %q, want %q", what, buf[:n], want)
	}
}

// signalOnce returns a channel and a function that closes it the first time it
// is called.
func signalOnce() (<-chan struct{}, func()) {
	ch := make(chan struct{})

	var once sync.Once

	return ch, func() { once.Do(func() { close(ch) }) }
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestEndMarkerLinkConnects(t *testing.T) {
	tests := []struct {
		name      string
		failures  int
		wantDials int
	}{
		{name: "socket there on the first try", failures: 0, wantDials: 1},
		{name: "socket appears after one failed dial", failures: 1, wantDials: 2},
		{name: "socket appears after several failed dials", failures: 5, wantDials: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newScriptedDatapath(tt.failures)
			markers, _ := startTestLink(t, dp.dial)

			conn := dp.accept(t, tt.name)
			expectEndMarker(t, markers, conn, []byte("marker"), tt.name)

			if got := dp.dialCount(); got != tt.wantDials {
				t.Errorf("dials: got %d, want %d", got, tt.wantDials)
			}
		})
	}
}

// The datapath restarting is seen as the connection dropping, and the link dials
// again without waiting for an End Marker to fail on the dead connection.
func TestEndMarkerLinkRedialsWhenTheConnectionDrops(t *testing.T) {
	tests := []struct {
		name  string
		drops int
	}{
		{name: "one drop", drops: 1},
		{name: "repeated drops", drops: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newScriptedDatapath(0)
			markers, _ := startTestLink(t, dp.dial)

			conn := dp.accept(t, "first connection")
			expectEndMarker(t, markers, conn, []byte("before"), "first connection")

			for i := range tt.drops {
				conn.Close()

				conn = dp.accept(t, "after the connection dropped")
				expectEndMarker(t, markers, conn, []byte{byte('a' + i)}, "after the connection dropped")
			}

			if got, want := dp.dialCount(), tt.drops+1; got != want {
				t.Errorf("dials: got %d, want %d", got, want)
			}
		})
	}
}

// bessd closes a second client as soon as it accepts it while it still holds the
// first, so a redial straight after a drop can be lost at once. A datapath that
// does that every time must not drive the link into a tight dial loop: every
// redial after a drop waits at least redialMin.
//
// The dial times are recorded rather than the gaps being guessed, and only lower
// bounds are checked, which a timer cannot undercut.
func TestEndMarkerLinkBacksOffWhenConnectionsAreLostAtOnce(t *testing.T) {
	const (
		redialMin = 20 * time.Millisecond
		drops     = 5
	)

	var (
		mu    sync.Mutex
		dials []time.Time
	)

	enough, signal := signalOnce()

	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		mu.Lock()
		dials = append(dials, time.Now())
		n := len(dials)
		mu.Unlock()

		if n > drops {
			signal()
			<-ctx.Done()

			return nil, ctx.Err()
		}

		// Accepted, then closed straight away, as bessd does to an extra client.
		agent, datapath := net.Pipe()
		datapath.Close()

		return agent, nil
	}

	startTestLink(t, dial, func(l *endMarkerLink) {
		l.redialMin = redialMin
		l.redialMax = 4 * redialMin
	})

	waitFor(t, enough, "the redials")

	mu.Lock()
	defer mu.Unlock()

	for i := 1; i < len(dials); i++ {
		if gap := dials[i].Sub(dials[i-1]); gap < redialMin {
			t.Errorf("redial %d came %v after the connection before it was lost; want at least %v", i, gap, redialMin)
		}
	}
}

// End Markers that arrive while the socket is unreachable are discarded, not
// held: held, they fill the bounded queue and block the PFCP handler, and they
// would reach the gNB after the path switch they belong to has completed.
func TestEndMarkerLinkDiscardsMarkersWhileUnreachable(t *testing.T) {
	gate := make(chan struct{})
	dp := newScriptedDatapath(0)

	gatedDial := func(ctx context.Context, network, address string) (net.Conn, error) {
		select {
		case <-gate:
			return dp.dial(ctx, network, address)
		default:
			return nil, errSocketMissing
		}
	}

	markers, _ := startTestLink(t, gatedDial)

	for range 64 {
		select {
		case markers <- []byte("stale"):
		case <-time.After(5 * time.Second):
			t.Fatal("the link stopped draining End Markers while the socket was unreachable")
		}
	}

	// Every stale marker has been taken off the queue while the dial was still
	// failing, so none of them can be sent once it succeeds. Markers still queued
	// at that moment would be sent, and that is not what this test is about.
	deadline := time.Now().Add(5 * time.Second)
	for len(markers) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d End Markers still queued while the socket was unreachable", len(markers))
		}

		time.Sleep(time.Millisecond)
	}

	close(gate)

	conn := dp.accept(t, "socket appeared")
	expectEndMarker(t, markers, conn, []byte("fresh"), "first marker after connecting")
}

func TestEndMarkerLinkStops(t *testing.T) {
	type stopCase struct {
		name string
		// dial is the datapath. It returns the channel to wait on before
		// stopping, closed once the link is in the state the case is about.
		setup func(t *testing.T) (dialFunc, <-chan struct{})
		// after runs once that state is reached, before stopping.
		after func(t *testing.T, markers chan<- []byte)
		tune  func(*endMarkerLink)
	}

	tests := []stopCase{
		{
			name: "while a dial is in flight",
			setup: func(t *testing.T) (dialFunc, <-chan struct{}) {
				entered, signal := signalOnce()

				return func(ctx context.Context, network, address string) (net.Conn, error) {
					signal()
					<-ctx.Done()

					return nil, ctx.Err()
				}, entered
			},
		},
		{
			// The wait is far longer than the test, so stopping has to cut it short.
			name: "while waiting to redial",
			setup: func(t *testing.T) (dialFunc, <-chan struct{}) {
				failed, signal := signalOnce()

				return func(ctx context.Context, network, address string) (net.Conn, error) {
					defer signal()
					return nil, errSocketMissing
				}, failed
			},
			tune: func(l *endMarkerLink) { l.redialMin, l.redialMax = time.Hour, time.Hour },
		},
		{
			name: "while connected",
			setup: func(t *testing.T) (dialFunc, <-chan struct{}) {
				dp := newScriptedDatapath(0)
				connected, signal := signalOnce()

				return func(ctx context.Context, network, address string) (net.Conn, error) {
					defer signal()
					return dp.dial(ctx, network, address)
				}, connected
			},
		},
		{
			// The datapath end of a pipe that nobody reads: the first write blocks.
			name: "while a write is blocked",
			setup: func(t *testing.T) (dialFunc, <-chan struct{}) {
				dp := newScriptedDatapath(0)
				connected, signal := signalOnce()

				return func(ctx context.Context, network, address string) (net.Conn, error) {
					defer signal()
					return dp.dial(ctx, network, address)
				}, connected
			},
			after: func(t *testing.T, markers chan<- []byte) {
				// The queue holds cap(markers); one more fits only once the
				// sender has taken a marker into its blocked write.
				for range cap(markers) + 1 {
					select {
					case markers <- []byte("marker"):
					case <-time.After(5 * time.Second):
						t.Fatal("the sender stopped taking markers before its write blocked")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dial, reached := tt.setup(t)

			var tune []func(*endMarkerLink)
			if tt.tune != nil {
				tune = append(tune, tt.tune)
			}

			markers, stopLink := startTestLink(t, dial, tune...)

			waitFor(t, reached, tt.name)

			if tt.after != nil {
				tt.after(t, markers)
			}

			select {
			case <-stopLink():
			case <-time.After(5 * time.Second):
				t.Fatal("the link did not stop")
			}
		})
	}
}

// Calling SetUpfInfo again stops the sender it started before, rather than
// leaving one that Exit cannot reach, and Exit waits for the sender it stops.
func TestSetUpfInfoTwiceLeavesNoEndMarkerSender(t *testing.T) {
	b := &bess{
		endMarkerDial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, errSocketMissing
		},
	}
	conf := &Conf{EnableEndMarker: true, EndMarkerSockAddr: "/test/pfcpport"}

	setUpfInfoWithoutDatapath(t, b, conf)

	first := b.endMarkerDone

	b.SetUpfInfo(&upf{}, conf)

	second := b.endMarkerDone

	select {
	case <-first:
	default:
		t.Fatal("the second SetUpfInfo left the first End Marker sender running")
	}

	b.Exit()

	select {
	case <-second:
	default:
		t.Fatal("Exit returned with the End Marker sender still running")
	}

	// The cleanup calls Exit again; stopping the End Marker sender twice is safe.
}
