// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/types"
)

// fragmentedIPv4 builds one fragment of an IPv4 datagram: the identification and the
// fragmentation fields the header carries, and a payload that is a transport header only in
// the first fragment.
//
// It exists because the existing builders produce whole datagrams, and the whole subject here
// is what a *later* fragment looks like: the bytes at the transport-header offsets are
// payload, and the offset field is the far end's to choose.
func fragmentedIPv4(src, dst string, proto uint8, ident uint16, offset uint16, more bool, payload []byte) []byte {
	p := make([]byte, 20, 20+len(payload))
	p[0] = 0x45 // version 4, 5-word header
	binary.BigEndian.PutUint16(p[2:4], uint16(20+len(payload)))
	binary.BigEndian.PutUint16(p[4:6], ident)

	flags := offset & 0x1fff
	if more {
		flags |= 0x2000
	}
	binary.BigEndian.PutUint16(p[6:8], flags)

	p[9] = proto
	copy(p[12:16], net.ParseIP(src).To4())
	copy(p[16:20], net.ParseIP(dst).To4())

	return append(p, payload...)
}

// uplinkFragment is a teed uplink copy carrying one fragment of the target's datagram.
func uplinkFragment(ident, offset uint16, more bool, payload []byte) []byte {
	return ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident, offset, more, payload))
}

// portFilter is the filter a transport-port criterion produces on rules that constrain no
// port, which is the only shape that has to read the packet at all.
func portFilter(t *testing.T) copyFilter {
	t.Helper()

	f := filterFor(taskWith(types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"}),
		wildcardSession())
	if f.trivial() {
		t.Fatal("a port criterion on wildcard rules cannot be trivial — nothing would be filtered")
	}
	if f.frags == nil {
		t.Fatal("a filter that has to read the packet was built without a fragment memo, so every " +
			"later fragment of an authorised datagram is dropped for not repeating a transport header")
	}

	return f
}

// TestEveryFragmentOfAnAuthorisedDatagramIsDelivered: a selected datagram is copied whole, not
// only its first fragment.
//
// A fragmented IPv4 datagram carries its transport header in the first fragment only, so
// every fragment after the first has no ports to read. Dropping those copies would give an
// agency the head of each datagram and nothing else — and downstream that is invisible,
// because the dropped copies never reach framing, so the X3 sequence has no gap in it to
// notice.
func TestEveryFragmentOfAnAuthorisedDatagramIsDelivered(t *testing.T) {
	f := portFilter(t)

	const ident = 0x1234

	// The initial fragment carries the transport header, and the criterion's port.
	first := uplinkFragment(ident, 0, true, tcpSegment(443, 80, 60))
	if !f.matches(farForwardUAndDuplicate, first) {
		t.Fatal("the initial fragment of an authorised datagram was dropped")
	}

	// Its continuation fragments carry payload where the ports would be. They belong to the
	// same datagram, so they belong to the same interception.
	for i, offset := range []uint16{8, 16, 24} {
		frag := uplinkFragment(ident, offset, offset != 24, []byte{0xde, 0xad, 0xbe, 0xef, byte(i)})
		if !f.matches(farForwardUAndDuplicate, frag) {
			t.Errorf("fragment at offset %d of an authorised datagram was dropped: the agency "+
				"receives the head of each datagram and nothing else, and the X3 sequence has no "+
				"gap in it to show that", offset)
		}
	}
}

// TestFragmentsOfANonMatchingDatagramAreStillDropped is the other direction. The decision
// taken from the first fragment applies to the datagram either way, so a datagram the
// criterion does not select must not become deliverable by being fragmented.
func TestFragmentsOfANonMatchingDatagramAreStillDropped(t *testing.T) {
	f := portFilter(t)

	const ident = 0x4321

	first := uplinkFragment(ident, 0, true, tcpSegment(8080, 80, 60))
	if f.matches(farForwardUAndDuplicate, first) {
		t.Fatal("a datagram on a port the criterion does not name was delivered")
	}

	frag := uplinkFragment(ident, 8, false, []byte{0x01, 0x02, 0x03, 0x04})
	if f.matches(farForwardUAndDuplicate, frag) {
		t.Error("a fragment of a datagram the criterion does not select was delivered: " +
			"fragmenting traffic must not make it collectable")
	}
}

// TestAFragmentBeforeItsFirstIsDroppedAndReported is the declared limit.
//
// Retaining out-of-order fragments until the one carrying the transport header arrives is a
// second mechanism on the one path in this element whose cost is per packet, in front of a
// datagram queue that holds ten by default, and it is reachable by a peer who can choose
// fragment order. So it is not done — and the loss it leaves is reported rather than silent,
// which is the difference between a stated limit and a defect.
func TestAFragmentBeforeItsFirstIsDroppedAndReported(t *testing.T) {
	// Built through filterFrom rather than filterFor, because the loss report is what is
	// being asserted and only this entry point takes the channel it goes on.
	c, err := parseCriterion(types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"})
	if err != nil {
		t.Fatalf("parseCriterion: %v", err)
	}

	var losses int
	f := filterFrom([]criterion{c}, wildcardSession(), func() { losses++ })
	if f.frags == nil {
		t.Fatal("no fragment memo was built")
	}

	// A continuation fragment arriving first: this element cannot know whether the datagram
	// was authorised.
	frag := uplinkFragment(0x5555, 8, true, []byte{0xaa, 0xbb, 0xcc, 0xdd})
	if f.matches(farForwardUAndDuplicate, frag) {
		t.Error("a fragment this element could not classify was delivered: delivering on a guess " +
			"is collection the warrant may not cover")
	}
	if losses != 1 {
		t.Errorf("a discarded fragment was reported %d times, want 1: a loss this element cannot "+
			"see is the one thing this plane may not produce", losses)
	}

	// A *known* non-match is not reported: dropping those fragments is what the criterion
	// asked for, and reporting them would bury the reports that mean something.
	losses = 0
	if f.matches(farForwardUAndDuplicate, uplinkFragment(0x6666, 0, true, tcpSegment(8080, 80, 60))) {
		t.Fatal("a non-matching datagram was delivered")
	}
	if f.matches(farForwardUAndDuplicate, uplinkFragment(0x6666, 8, false, []byte{0x01})) {
		t.Fatal("a fragment of a non-matching datagram was delivered")
	}
	if losses != 0 {
		t.Errorf("dropping the fragments of a datagram the criterion does not select was reported "+
			"%d times as content loss", losses)
	}
}

// TestAnIdentificationCollisionDoesNotShareAClassification: the identification field is
// chosen by the sender and is unique only per (source, destination, protocol), so a memo
// keyed by it alone would apply one datagram's classification to another's fragments — either
// delivering traffic the warrant does not name, or dropping traffic it does.
//
// Every scope field is exercised, because a key missing one of them passes a test that varies
// only the others.
func TestAnIdentificationCollisionDoesNotShareAClassification(t *testing.T) {
	const ident = 0x7777

	for _, tc := range []struct {
		name  string
		later []byte
		// asFirst is the matching datagram whose classification must not be reused.
		asFirst []byte
	}{
		{
			name:    "a different source address",
			asFirst: ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident, 0, true, tcpSegment(443, 80, 60))),
			later:   ethIPv4(fragmentedIPv4("10.250.0.10", "1.1.1.1", protoTCP, ident, 8, false, []byte{0x01})),
		},
		{
			name:    "a different destination address",
			asFirst: ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident, 0, true, tcpSegment(443, 80, 60))),
			later:   ethIPv4(fragmentedIPv4(testUEIPv4, "2.2.2.2", protoTCP, ident, 8, false, []byte{0x01})),
		},
		{
			name:    "a different protocol",
			asFirst: ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident, 0, true, tcpSegment(443, 80, 60))),
			later:   ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoUDP, ident, 8, false, []byte{0x01})),
		},
		{
			name:    "a different identification",
			asFirst: ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident, 0, true, tcpSegment(443, 80, 60))),
			later:   ethIPv4(fragmentedIPv4(testUEIPv4, "1.1.1.1", protoTCP, ident+1, 8, false, []byte{0x01})),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := portFilter(t)

			if !f.matches(farForwardUAndDuplicate, tc.asFirst) {
				t.Fatal("the authorised datagram's first fragment was dropped")
			}
			if f.matches(farForwardUAndDuplicate, tc.later) {
				t.Error("a fragment of a different datagram was delivered on another datagram's " +
					"classification: the identification field is the sender's to choose and is " +
					"unique only per source, destination and protocol")
			}
		})
	}

	// And the direction, which shares the same header fields on the two legs of one session.
	t.Run("a different direction", func(t *testing.T) {
		f := portFilter(t)

		if !f.matches(farForwardUAndDuplicate, uplinkFragment(ident, 0, true, tcpSegment(443, 80, 60))) {
			t.Fatal("the uplink datagram's first fragment was dropped")
		}
		// The same identity, downlink: a different datagram, and undecided.
		down := ethIPv4(gtpuEncap(
			fragmentedIPv4("1.1.1.1", testUEIPv4, protoTCP, ident, 8, false, []byte{0x01}), 0, nil))
		if f.matches(farForwardDAndDuplicate, down) {
			t.Error("a downlink fragment was delivered on an uplink datagram's classification")
		}
	})
}

// TestAnExpiredClassificationIsDroppedSilently: a decision older than a datagram can
// plausibly still be arriving under is dropped, and dropping it reports nothing.
//
// Nothing records that a datagram's last fragment has passed, so an expired match is far more
// often a datagram delivered whole than one still arriving. Reporting it would tell the ADMF
// that every fragmented datagram this element delivered was lost. A fragment that does arrive
// after expiry is dropped and reported when it is discarded, which
// TestALateFragmentAfterExpiryIsDroppedAndReportedOnce pins.
func TestAnExpiredClassificationIsDroppedSilently(t *testing.T) {
	var losses int
	m := newFragmentMemo(func() { losses++ })

	now := time.Now()
	m.now = func() time.Time { return now }

	// Enough entries that the sweep runs, all of them matches.
	for i := range 20 {
		m.classify(fragKey{ident: uint16(i), proto: protoTCP}, true)
	}

	// Past the window, and one more classification to drive the sweep.
	now = now.Add(2 * fragmentMemoTTL)
	m.classify(fragKey{ident: 0xffff, proto: protoTCP}, true)

	if losses != 0 {
		t.Errorf("expiring %d classifications reported %d losses, want 0: no fragment was "+
			"discarded, and a datagram delivered whole is not content loss", 20, losses)
	}
	if _, known := m.decision(fragKey{ident: 0, proto: protoTCP}); known {
		t.Error("an expired classification is still held, so the memo grows with traffic")
	}
}

// TestALateFragmentAfterExpiryIsDroppedAndReportedOnce is the guarantee the silent expiry
// relies on: a continuation fragment that arrives after its datagram's classification has
// expired finds nothing, is discarded, and is reported — once, at the point it is discarded.
func TestALateFragmentAfterExpiryIsDroppedAndReportedOnce(t *testing.T) {
	c, err := parseCriterion(types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"})
	if err != nil {
		t.Fatalf("parseCriterion: %v", err)
	}

	var losses int
	f := filterFrom([]criterion{c}, wildcardSession(), func() { losses++ })
	if f.frags == nil {
		t.Fatal("no fragment memo was built")
	}

	now := time.Now()
	f.frags.now = func() time.Time { return now }

	const ident = 0x2468

	if !f.matches(farForwardUAndDuplicate, uplinkFragment(ident, 0, true, tcpSegment(443, 80, 60))) {
		t.Fatal("the initial fragment of an authorised datagram was dropped")
	}

	now = now.Add(2 * fragmentMemoTTL)

	if f.matches(farForwardUAndDuplicate, uplinkFragment(ident, 8, false, []byte{0x01})) {
		t.Error("a fragment arriving after its classification expired was delivered: this " +
			"element no longer knows whether it was authorised")
	}
	if losses != 1 {
		t.Errorf("a fragment discarded after expiry was reported %d times, want 1", losses)
	}
}

// TestTheMemoIsBounded: the ceiling is reached only by a peer fragmenting far more than a
// session normally does — which is also the shape of an attempt to make this element
// allocate.
//
// Refusing a classification at the ceiling reports nothing, because nothing has been
// discarded yet: the refused datagram's later fragments are dropped and reported when they
// arrive, and one with no later fragment loses nothing.
func TestTheMemoIsBounded(t *testing.T) {
	t.Parallel()

	var losses int
	m := newFragmentMemo(func() { losses++ })

	for i := range fragmentMemoMax + 100 {
		m.classify(fragKey{ident: uint16(i % 65535), proto: protoTCP, src: uint32(i)}, true)
	}

	m.mu.Lock()
	held := len(m.decided)
	m.mu.Unlock()

	if held > fragmentMemoMax {
		t.Errorf("the memo holds %d classifications against a ceiling of %d", held, fragmentMemoMax)
	}
	if losses != 0 {
		t.Errorf("refusing classifications at the ceiling reported %d losses, want 0: no "+
			"fragment was discarded, and the ones that will be are reported when they are", losses)
	}
}

// TestTheMemoIsSweptAtMostOncePerInterval: a sweep visits every entry, so one on every
// initial fragment walks a full memo — up to fragmentMemoMax comparisons — per datagram, on
// the session's one framing worker. Sweeps are at least fragmentMemoSweepEvery apart, and an
// entry past its TTL but not yet swept is still not used.
func TestTheMemoIsSweptAtMostOncePerInterval(t *testing.T) {
	t.Parallel()

	m := newFragmentMemo(nil)

	start := time.Now()
	now := start
	m.now = func() time.Time { return now }

	held := func() int {
		m.mu.Lock()
		defer m.mu.Unlock()

		return len(m.decided)
	}

	const old = 16
	for i := range old {
		m.classify(fragKey{ident: uint16(i), proto: protoTCP}, true)
	}

	// A sweep just before the old entries expire: it removes nothing, and the next is due
	// an interval later.
	now = start.Add(fragmentMemoTTL - fragmentMemoSweepEvery/2)
	m.classify(fragKey{ident: 100, proto: protoTCP}, true)

	// The old entries have expired, but the next sweep is not due yet.
	now = start.Add(fragmentMemoTTL + fragmentMemoSweepEvery/4)
	m.classify(fragKey{ident: 101, proto: protoTCP}, true)

	if got := held(); got != old+2 {
		t.Errorf("the memo holds %d classifications a quarter-interval after a sweep, want %d: "+
			"it was swept again, so a full memo is walked for every datagram", got, old+2)
	}
	if _, known := m.decision(fragKey{ident: 0, proto: protoTCP}); known {
		t.Error("an expired classification awaiting the next sweep was used")
	}

	// Once the interval has passed, the next classification sweeps the expired entries out.
	now = start.Add(fragmentMemoTTL + fragmentMemoSweepEvery)
	m.classify(fragKey{ident: 102, proto: protoTCP}, true)

	// The lookup above deleted ident 0 itself, so 15 old entries were left for the sweep.
	if got := held(); got != 3 {
		t.Errorf("the memo holds %d classifications after the interval, want 3: the sweep "+
			"no longer runs", got)
	}
}

// TestAFullMemoStillReplacesAnExistingClassification: the ceiling refuses a datagram that
// needs a new slot, not one whose key is already held.
//
// The identification field is the sender's to reuse, so a new first fragment can arrive with
// the key of a datagram classified moments earlier. Refusing it at the ceiling would leave the
// earlier verdict in place: a stale match delivers the new datagram's fragments outside the
// task's criteria, and a stale non-match drops them without a report.
func TestAFullMemoStillReplacesAnExistingClassification(t *testing.T) {
	m := newFragmentMemo(nil)

	now := time.Now()
	m.now = func() time.Time { return now }

	for i := range fragmentMemoMax {
		m.classify(fragKey{ident: uint16(i % 65535), proto: protoTCP, src: uint32(i)}, true)
	}

	existing := fragKey{ident: 7, proto: protoTCP, src: 7}
	m.classify(existing, false)

	if matched, known := m.decision(existing); matched || !known {
		t.Errorf("a full memo kept a reused key's earlier verdict: decision = (%v, %v), want "+
			"(false, true) — the new datagram's fragments follow the old one's classification",
			matched, known)
	}

	fresh := fragKey{ident: 1, proto: protoTCP, src: fragmentMemoMax + 1}
	m.classify(fresh, true)
	if _, known := m.decision(fresh); known {
		t.Error("a full memo recorded a new key: the ceiling no longer bounds it")
	}

	m.mu.Lock()
	held := len(m.decided)
	m.mu.Unlock()

	if held != fragmentMemoMax {
		t.Errorf("the memo holds %d classifications, want the ceiling of %d", held, fragmentMemoMax)
	}
}

// TestTheFragmentMemoIsConcurrent: framing runs four workers deep and they share the cached
// filter, so they share the memo. Run under -race.
func TestTheFragmentMemoIsConcurrent(t *testing.T) {
	t.Parallel()

	f := portFilter(t)

	const workers, each = 8, 200

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()

			for i := range each {
				ident := uint16(w*each + i)
				f.matches(farForwardUAndDuplicate, uplinkFragment(ident, 0, true, tcpSegment(443, 80, 60)))
				f.matches(farForwardUAndDuplicate, uplinkFragment(ident, 8, false, []byte{0x01}))
			}
		}(w)
	}
	wg.Wait()
}

// TestUnfragmentedTrafficAllocatesNothing is the cost property, and it is why the memo is
// only consulted for a fragmented datagram.
//
// This is the one path in the element whose cost is per packet. A mechanism that allocated
// per copy would be paid for by every intercepted session, for a case — fragmentation — that
// most traffic does not produce at all.
func TestUnfragmentedTrafficAllocatesNothing(t *testing.T) {
	f := portFilter(t)
	frame := uplinkCopy(443)

	if !f.matches(farForwardUAndDuplicate, frame) {
		t.Fatal("the unfragmented case does not match, so this measures the wrong path")
	}

	allocs := testing.AllocsPerRun(200, func() {
		f.matches(farForwardUAndDuplicate, frame)
	})
	if allocs != 0 {
		t.Errorf("deciding an unfragmented copy allocated %.0f times, want 0: this is the "+
			"per-packet path, and the fragment machinery must cost nothing on traffic that is "+
			"not fragmented", allocs)
	}
}
