// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"sync"
	"testing"
	"time"
)

// trackedConn reports whether it was closed, which is the only thing that matters here: a
// connection nobody closes is one the listener stays parked on after shutdown.
type trackedConn struct {
	net.Conn

	mu     sync.Mutex
	closed bool
}

func (c *trackedConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	return c.Conn.Close()
}

func (c *trackedConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}

func newTrackedConn(t *testing.T) *trackedConn {
	t.Helper()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	return &trackedConn{Conn: client}
}

// A connection dialled before shutdown is adopted, and Exit is then able to find and close it.
func TestNotifyConnAdoptedBeforeShutdownIsClosedByExit(t *testing.T) {
	b := &bess{notifyStop: make(chan struct{})}
	conn := newTrackedConn(t)

	if !b.adoptNotifyConn(conn) {
		t.Fatal("a connection dialled before shutdown must be adopted")
	}

	close(b.notifyStop)
	b.closeNotifyConn()

	if !conn.isClosed() {
		t.Error("the adopted connection was not closed on shutdown; the listener would park on it")
	}
}

// A connection that arrives after shutdown has begun is refused, so the caller closes it rather
// than registering it into a slot that has already been swept.
func TestNotifyConnIsRefusedAfterShutdownHasBegun(t *testing.T) {
	b := &bess{notifyStop: make(chan struct{})}

	// The order Exit uses: close the channel, then sweep the slot.
	close(b.notifyStop)
	b.closeNotifyConn()

	conn := newTrackedConn(t)
	if b.adoptNotifyConn(conn) {
		t.Fatal("a connection dialled after shutdown must be refused; nothing will close it")
	}
	if b.notifyConn != nil {
		t.Error("a refused connection must not be registered")
	}
}

// The interleaving the check-then-register version got wrong, driven deterministically rather
// than hoped for: shutdown happens while the listener is already past its stop check and waiting
// to register.
//
// Holding notifyMu is what makes it reproducible. The listener goroutine parks on the lock; the
// stop channel is then closed and the slot swept while it waits, which is exactly Exit's order;
// and only then is the lock released. A stop check taken before the lock has already read false by
// that point, so the connection would be registered into a slot nothing will sweep again. Taking
// the check inside the lock is what makes the second read the one that counts.
func TestNotifyConnIsRefusedWhenShutdownHappensWhileWaitingToRegister(t *testing.T) {
	b := &bess{notifyStop: make(chan struct{})}
	conn := newTrackedConn(t)

	b.notifyMu.Lock()

	adopted := make(chan bool, 1)

	go func() {
		if !b.adoptNotifyConn(conn) {
			conn.Close() // what the listener does with a refused connection
		} else {
			adopted <- true
		}

		close(adopted)
	}()

	// Let the goroutine reach adoptNotifyConn and park on the lock. Any stop check outside the
	// lock has been evaluated by now, and it read false.
	time.Sleep(50 * time.Millisecond)

	// Exit's order, with the lock held so the listener cannot interleave: signal, then sweep.
	close(b.notifyStop)
	b.notifyConn = nil

	b.notifyMu.Unlock()

	if <-adopted {
		t.Fatal("the connection was adopted after shutdown had begun and the slot had been swept; " +
			"nothing will close it and the listener parks on it for good")
	}

	if b.notifyConn != nil {
		t.Error("a refused connection must not be registered")
	}

	if !conn.isClosed() {
		t.Error("a refused connection must be closed by its caller")
	}
}
