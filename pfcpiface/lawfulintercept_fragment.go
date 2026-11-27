// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/binary"
	"sync"
	"time"
)

// A transport-port criterion has to read the packet, and a fragmented IPv4 datagram carries
// its transport header in the first fragment only. So every fragment after the first was
// unreadable, the port comparison reported false, and the copy was dropped — **every non-initial
// fragment of a datagram this element had just decided was authorised**. An agency receives
// the head of each datagram and nothing else, which downstream is not a gap it can see: the
// X3 sequence is unbroken, because the dropped copies were dropped before framing.
//
// The remedy is to classify a datagram from its initial fragment and apply that decision to
// the datagram's later fragments. What is *not* done is holding fragments that arrive before
// the one carrying the transport header: that is a second mechanism on the one path in this
// element whose cost is per packet, in front of a datagram queue that holds ten by default,
// and it is reachable by a peer who can choose fragment order — a denial-of-service surface
// added to recover the rarest arrival order. IPv4 fragments of one datagram normally
// arrive in order over a GTP-U path, so classify-on-first recovers nearly all of the loss for
// a fraction of the mechanism. The limit is declared rather than discovered: a fragment
// arriving before its datagram's first is discarded and reported as content loss.

const (
	// fragmentMemoTTL is how long a classification is kept. It bounds the memo by the time a
	// datagram can plausibly still be arriving rather than by traffic: IPv4 reassembly
	// timeouts are of this order, and a decision older than that belongs to a datagram no
	// receiver is still assembling.
	fragmentMemoTTL = 15 * time.Second

	// fragmentMemoMax is how many classifications one filter may hold. Reached only by a
	// peer fragmenting far more than a session normally does, which is also the shape of an
	// attempt to make this element allocate — so it is a ceiling and not a target. Nothing
	// is evicted: a datagram arriving at the ceiling is not recorded, and its later fragments
	// are dropped and reported as they are discarded, so the bound costs visibility rather
	// than silence.
	fragmentMemoMax = 4096

	// fragmentMemoSweepEvery is how often classify looks for expired classifications once
	// the memo is worth sweeping. A sweep visits every entry, so sweeping on every initial
	// fragment would cost a full memo — up to fragmentMemoMax comparisons — per datagram at
	// the ceiling, on the session's one framing worker. Once a second bounds that to one
	// pass a second whatever the fragment rate. An expired entry can therefore outlive its
	// TTL by up to this long, which decision does not see, since it checks the age of what
	// it finds; at the ceiling such an entry holds its slot that much longer.
	fragmentMemoSweepEvery = time.Second
)

// fragKey identifies one IPv4 datagram, within one direction of one session's filter.
//
// The session and the task generation are not in the key because they are in the *memo*: a
// memo belongs to one cached copyFilter, which is held per session under the tasking epoch
// and dropped when either changes. Getting it from the
// memo's lifetime rather than from the key means a generation change cannot leave a
// classification behind to be reused across lifecycles.
//
// The identification field alone is not enough — it is chosen by the sender and is only
// unique per (source, destination, protocol) — so a collision would apply one datagram's
// classification to another's fragments. All four travel.
type fragKey struct {
	uplink   bool
	src, dst uint32
	proto    uint8
	ident    uint16
}

// fragDecision is what the initial fragment settled, and when.
type fragDecision struct {
	matched bool
	at      time.Time
}

// fragmentMemo holds the classification of each fragmented datagram in flight.
//
// Allocated only for a filter that has an inspecting arm — a transport-port criterion whose
// rule does not pin both the port and the protocol — so a task keyed by a session, an address
// or a tunnel pays nothing and the unfragmented path allocates nothing at all.
//
// Held per session, and a session is framed by one worker (see frameQueueFor), so the lock
// is not there for concurrent framing: a tasking change rebuilds the memo while a worker may
// be using it. The map is small and the critical section is a lookup, which is why a mutex
// rather than anything cleverer.
type fragmentMemo struct {
	// onLoss reports that a fragment of a datagram this element could not classify was
	// discarded. It is the existing X3 content-loss condition, not a new one: from the
	// agency's side a fragment dropped here is a copy this element made and did not deliver.
	onLoss func()

	mu      sync.Mutex
	decided map[fragKey]fragDecision
	// nextSweep is the earliest time classify sweeps again. See fragmentMemoSweepEvery.
	nextSweep time.Time
	// now is the clock, held so a test can move it rather than wait out the TTL.
	now func() time.Time
}

func newFragmentMemo(onLoss func()) *fragmentMemo {
	return &fragmentMemo{
		onLoss:  onLoss,
		decided: make(map[fragKey]fragDecision, 16),
		now:     time.Now,
	}
}

// classify records what the initial fragment of a datagram settled.
func (m *fragmentMemo) classify(k fragKey, matched bool) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()

	// Expiry is swept here rather than on a timer, so the memo has no goroutine of its own
	// and holds nothing when traffic stops. A datagram whose classification has expired is
	// one no receiver is still assembling.
	//
	// The sweep reports nothing. Nothing records that a datagram's last fragment has
	// passed, so an expired entry is far more often a datagram delivered whole than one
	// still arriving — and one still arriving loses nothing here: its next fragment finds
	// no classification, is dropped, and is reported then, which is where the loss is.
	//
	// Swept only when the map is worth sweeping, so an ordinary session with a handful of
	// datagrams in flight pays one comparison, and at most once per fragmentMemoSweepEvery,
	// so a full memo is not walked for every datagram.
	if len(m.decided) >= 16 && !now.Before(m.nextSweep) {
		cutoff := now.Add(-fragmentMemoTTL)
		for key, d := range m.decided {
			if d.at.Before(cutoff) {
				delete(m.decided, key)
			}
		}
		m.nextSweep = now.Add(fragmentMemoSweepEvery)
	}

	if _, exists := m.decided[k]; len(m.decided) >= fragmentMemoMax && !exists {
		// The ceiling. Rather than evicting an arbitrary entry — which would drop a
		// classification still in use — the new datagram is simply not recorded, and nothing
		// is reported yet: no fragment has been discarded. Its later fragments will find
		// nothing, be dropped, and be reported at that point, which is where the loss
		// actually is; a datagram with no later fragment loses nothing.
		//
		// A key already held needs no new slot, so it is replaced rather than refused:
		// keeping it would apply an earlier datagram's verdict to this one's fragments.
		return
	}

	m.decided[k] = fragDecision{matched: matched, at: now}
}

// decision returns what the datagram's initial fragment settled, and whether this element
// has one at all.
func (m *fragmentMemo) decision(k fragKey) (bool, bool) {
	if m == nil {
		return false, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.decided[k]
	if !ok {
		return false, false
	}
	if m.now().Sub(d.at) > fragmentMemoTTL {
		delete(m.decided, k)

		return false, false
	}

	return d.matched, true
}

// fragmentOf reads an IPv4 header's fragmentation state and the datagram's identity.
//
// It reports false for anything that is not an IPv4 header it can read, which is the same
// test transportOf applies — the two are deliberately the same shape, because a header this
// element cannot parse must not be classified either way.
func fragmentOf(l3 []byte, uplink bool) (key fragKey, offset uint16, more bool, ok bool) {
	const (
		minIPv4Header = 20
		identOffset   = 4
		flagsOffset   = 6
		protoOffset   = 9
		srcOffset     = 12
		dstOffset     = 16
		fragmentMask  = 0x1fff
		moreFragments = 0x2000
	)

	if len(l3) < minIPv4Header || l3[0]>>4 != 4 {
		return fragKey{}, 0, false, false
	}

	flags := binary.BigEndian.Uint16(l3[flagsOffset : flagsOffset+2])

	return fragKey{
		uplink: uplink,
		src:    binary.BigEndian.Uint32(l3[srcOffset : srcOffset+4]),
		dst:    binary.BigEndian.Uint32(l3[dstOffset : dstOffset+4]),
		proto:  l3[protoOffset],
		ident:  binary.BigEndian.Uint16(l3[identOffset : identOffset+2]),
	}, flags & fragmentMask, flags&moreFragments != 0, true
}
