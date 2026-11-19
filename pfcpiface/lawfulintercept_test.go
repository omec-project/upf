// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
	"github.com/wmnsk/go-pfcp/ie"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestParseFARDuplicate checks that a FAR carrying the DUPL apply-action plus
// Duplicating Parameters is recognised for Lawful Interception, and that a plain
// forwarding FAR is not.
func TestParseFARDuplicate(t *testing.T) {
	mockUpf := &upf{accessIP: net.ParseIP("192.168.0.1"), coreIP: net.ParseIP("10.0.10.1")}

	dupFAR := &far{}
	in := ie.NewCreateFAR(
		ie.NewFARID(7),
		ie.NewApplyAction(ActionForward|ActionDuplicate),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
		ie.NewDuplicatingParameters(ie.NewDestinationInterface(ie.DstInterfaceLIFunction)),
	)
	if err := dupFAR.parseFAR(in, 100, mockUpf, create); err != nil {
		t.Fatalf("parseFAR (dup): %v", err)
	}
	if !dupFAR.Duplicates() {
		t.Errorf("DUPL FAR: Duplicates() = false, want true")
	}
	if !dupFAR.Forwards() {
		t.Error("DUPL FAR must still forward the subscriber copy")
	}

	plainFAR := &far{}
	plain := ie.NewCreateFAR(
		ie.NewFARID(8),
		ie.NewApplyAction(ActionForward),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
	)
	if err := plainFAR.parseFAR(plain, 100, mockUpf, create); err != nil {
		t.Fatalf("parseFAR (plain): %v", err)
	}
	if plainFAR.Duplicates() {
		t.Error("plain forwarding FAR must not be marked for duplication")
	}
}

// TestParseFARDuplicateIsSilent enforces undetectability: parsing a DUPL (LI)
// FAR must emit no log output, so a tasked subscriber's session is
// indistinguishable from any other in the UPF's logs.
func TestParseFARDuplicateIsSilent(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	orig := logger.PfcpLog
	logger.PfcpLog = zap.New(core).Sugar()
	t.Cleanup(func() { logger.PfcpLog = orig })

	mockUpf := &upf{accessIP: net.ParseIP("192.168.0.1"), coreIP: net.ParseIP("10.0.10.1")}
	dup := &far{}
	in := ie.NewCreateFAR(
		ie.NewFARID(7),
		ie.NewApplyAction(ActionForward|ActionDuplicate),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceAccess)),
		ie.NewDuplicatingParameters(ie.NewDestinationInterface(ie.DstInterfaceLIFunction)),
	)
	if err := dup.parseFAR(in, 100, mockUpf, create); err != nil {
		t.Fatalf("parseFAR: %v", err)
	}
	if n := logs.Len(); n != 0 {
		t.Errorf("parsing a DUPL FAR emitted %d log entries, want 0 (undetectability): %v", n, logs.All())
	}
}

// TestSetFwdActionValueDuplicate checks that DUPL forwarding FARs are routed to
// the LI tee, and — the part that matters — that this never reaches the "action"
// attribute.
//
// BESS's GtpuEncap reuses "action" as the GTP-U PDU Session Container PDU Type,
// where only 0 (downlink) and 1 (uplink) exist. Putting a duplication value there
// would ship an undefined PDU type to the RAN. The duplication variants therefore
// belong on fwd_action alone, and the second half of this table guards that.
func TestSetFwdActionValueDuplicate(t *testing.T) {
	b := &bess{}

	dlDup := far{applyAction: ActionForward | ActionDuplicate, dstIntf: ie.DstInterfaceAccess}
	ulDup := far{applyAction: ActionForward | ActionDuplicate, dstIntf: ie.DstInterfaceCore}
	dlPlain := far{applyAction: ActionForward, dstIntf: ie.DstInterfaceAccess}
	ulPlain := far{applyAction: ActionForward, dstIntf: ie.DstInterfaceCore}
	// Toward another UPF over N9: the FAR creates an outer header, so GtpuEncap runs before
	// executeFAR and the tee sees the GTP-U packet. Its own value tells the shipper so.
	ulN9Dup := far{applyAction: ActionForward | ActionDuplicate, dstIntf: ie.DstInterfaceCore, tunnelType: 1}
	ulN9Plain := far{applyAction: ActionForward, dstIntf: ie.DstInterfaceCore, tunnelType: 1}

	// What executeFAR splits on: duplication is visible here.
	for _, c := range []struct {
		name   string
		f      far
		expect uint8
	}{
		{"downlink+dupl", dlDup, farForwardDAndDuplicate},
		{"uplink+dupl", ulDup, farForwardUAndDuplicate},
		{"N9 uplink+dupl", ulN9Dup, farForwardUEncapAndDuplicate},
		{"downlink plain", dlPlain, farForwardD},
		{"uplink plain", ulPlain, farForwardU},
		{"N9 uplink plain", ulN9Plain, farForwardU},
	} {
		if got := b.setFwdActionValue(c.f); got != c.expect {
			t.Errorf("%s: fwd_action = %d, want %d", c.name, got, c.expect)
		}
	}

	// What GtpuEncap consumes as the PSC PDU type: duplication must be invisible,
	// and the value must stay one of the two the wire format defines.
	for _, c := range []struct {
		name   string
		f      far
		expect uint8
	}{
		{"downlink+dupl stays a valid PDU type", dlDup, farForwardD},
		{"uplink+dupl stays a valid PDU type", ulDup, farForwardU},
		{"N9 uplink+dupl stays a valid PDU type", ulN9Dup, farForwardU},
		{"downlink plain", dlPlain, farForwardD},
		{"uplink plain", ulPlain, farForwardU},
	} {
		if got := b.setActionValue(c.f); got != c.expect {
			t.Errorf("%s: action = %d, want %d (0 and 1 are the only defined PDU types)", c.name, got, c.expect)
		}
	}
}

func TestPayloadFormatOf(t *testing.T) {
	if got := payloadFormatOf([]byte{0x45, 0x00}); got != x2x3.PayloadFormatIPv4 {
		t.Errorf("IPv4 packet → %d, want %d", got, x2x3.PayloadFormatIPv4)
	}
	if got := payloadFormatOf([]byte{0x60, 0x00}); got != x2x3.PayloadFormatIPv6 {
		t.Errorf("IPv6 packet → %d, want %d", got, x2x3.PayloadFormatIPv6)
	}
	if got := payloadFormatOf(nil); got != x2x3.PayloadFormatIPv4 {
		t.Errorf("empty packet → %d, want IPv4 default", got)
	}
}

// TestShipperPDU checks that a teed datapath packet ([fseid(8)][action(1)][pkt])
// is framed as a valid X3 PDU: the warrant XID and correlation identifier taken
// from the session's LI_T3 task, the inner packet as payload, and — critically
// — the payload format + direction set from the FAR action, so the
// downlink copy is labeled GTP-U (not mislabeled as decapsulated inner IP) and
// the uplink copy is labeled inner IP.
func TestShipperPDU(t *testing.T) {
	fseid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	// The identity a CC-TF supplied for this session. Neither field is derived
	// from the packet: the XID is the warrant's, the correlation is the value the
	// SMF also put on the session's xIRI.
	task := types.InterceptTask{
		XID:           testXIDPrimary,
		ProductID:     testXIDSEIDShaped,
		CorrelationID: 0x2632898145f4d191,
	}
	inner := []byte{0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	tag := func(action byte) []byte {
		return append(append(append([]byte{}, fseid...), action), inner...)
	}

	// Uplink (action 6): decapsulated inner IP → IPv4 + FromTarget.
	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	ul := shipperPDU(tag(farForwardUAndDuplicate), task, ids)
	if ul.Type != x2x3.PDUTypeX3 {
		t.Errorf("PDU type = %d, want X3", ul.Type)
	}
	if ul.PayloadFormat != x2x3.PayloadFormatIPv4 || ul.Direction != x2x3.DirectionFromTarget {
		t.Errorf("uplink: format=%d direction=%d, want IPv4/FromTarget", ul.PayloadFormat, ul.Direction)
	}
	// The correlation identifier is the task's, big-endian, so it matches the value
	// the SMF put on that session's X2 xIRI. It is not derived from the
	// datapath tag — the tag only selects the task.
	wantCorr := []byte{0x26, 0x32, 0x89, 0x81, 0x45, 0xf4, 0xd1, 0x91}
	if !bytes.Equal(ul.CorrelationID[:], wantCorr) || !bytes.Equal(ul.Payload, inner) {
		t.Errorf("uplink: correlation=% x (want % x) payload=% x", ul.CorrelationID, wantCorr, ul.Payload)
	}
	// The XID is the warrant's, taken from the task's ProductID. A zero XID here is
	// content no mediation function can attribute.
	wantXID := task.ProductID.Bytes()
	if ul.XID != wantXID {
		t.Errorf("uplink: XID = %x, want the warrant XID %x", ul.XID, wantXID)
	}
	if ul.XID == ([16]byte{}) {
		t.Error("uplink: XID is zero — the content would be unattributable")
	}

	// Downlink (action 5): teed post-encap → GTP-U + ToTarget (must NOT be labeled inner IP).
	dl := shipperPDU(tag(farForwardDAndDuplicate), task, ids)
	if dl.PayloadFormat != x2x3.PayloadFormatGTPU || dl.Direction != x2x3.DirectionToTarget {
		t.Errorf("downlink: format=%d direction=%d, want GTPU/ToTarget", dl.PayloadFormat, dl.Direction)
	}

	// Uplink toward another UPF (action 7): teed after the N9 encapsulation, so the
	// copy is a GTP-U packet from the target -> GTP-U + FromTarget.
	n9 := shipperPDU(tag(farForwardUEncapAndDuplicate), task, ids)
	if n9.PayloadFormat != x2x3.PayloadFormatGTPU || n9.Direction != x2x3.DirectionFromTarget {
		t.Errorf("N9 uplink: format=%d direction=%d, want GTPU/FromTarget", n9.PayloadFormat, n9.Direction)
	}

	// All must be well-formed X3 PDUs on the wire.
	for _, pdu := range []*x2x3.PDU{ul, dl, n9} {
		if _, err := pdu.Marshal(); err != nil {
			t.Errorf("Marshal: %v", err)
		}
	}
}

// TestShipperPDUStripsLinkLayer covers what the datapath actually tees: BESS's
// GTP-U decap leaves the Ethernet header in place, so the teed copy is a full
// frame (observed on a live bessd pipeline). X3 carries the network
// layer, so the L2 header must be stripped — otherwise the MDF receives 14 extra
// bytes under a payload format claiming an IP packet.
func TestShipperPDUStripsLinkLayer(t *testing.T) {
	ip := []byte{0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	ethHeader := func(etherType ...byte) []byte {
		hdr := []byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 2}

		return append(hdr, etherType...)
	}
	tag := func(action byte, payload []byte) []byte {
		return append(append(append([]byte{}, 1, 2, 3, 4, 5, 6, 7, 8), action), payload...)
	}

	tests := []struct {
		name       string
		action     byte
		payload    []byte
		wantFormat x2x3.PayloadFormat
		wantBody   []byte
	}{
		{
			name:       "ethernet framed uplink is stripped",
			action:     farForwardUAndDuplicate,
			payload:    append(ethHeader(0x08, 0x00), ip...),
			wantFormat: x2x3.PayloadFormatIPv4,
			wantBody:   ip,
		},
		{
			name:       "ethernet framed downlink is stripped and labelled GTP-U",
			action:     farForwardDAndDuplicate,
			payload:    append(ethHeader(0x08, 0x00), ip...),
			wantFormat: x2x3.PayloadFormatGTPU,
			wantBody:   ip,
		},
		{
			name:       "vlan tagged frame is stripped",
			action:     farForwardUAndDuplicate,
			payload:    append(append(ethHeader(0x81, 0x00), 0x00, 0x0a, 0x08, 0x00), ip...),
			wantFormat: x2x3.PayloadFormatIPv4,
			wantBody:   ip,
		},
		{
			name:       "bare ip packet is shipped unchanged",
			action:     farForwardUAndDuplicate,
			payload:    ip,
			wantFormat: x2x3.PayloadFormatIPv4,
			wantBody:   ip,
		},
		{
			name:       "unrecognised payload is labelled ethernet, not mislabelled as ip",
			action:     farForwardUAndDuplicate,
			payload:    append(ethHeader(0x12, 0x34), 0xaa, 0xbb),
			wantFormat: x2x3.PayloadFormatEthernet,
			wantBody:   append(ethHeader(0x12, 0x34), 0xaa, 0xbb),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdu := shipperPDU(tag(tt.action, tt.payload), types.InterceptTask{
				ProductID:     testXIDSEIDShaped,
				CorrelationID: 1,
			}, x2x3.NewIdentity(testNEID, upfInterceptionPoint))
			if pdu.PayloadFormat != tt.wantFormat {
				t.Errorf("payload format = %d, want %d", pdu.PayloadFormat, tt.wantFormat)
			}
			if !bytes.Equal(pdu.Payload, tt.wantBody) {
				t.Errorf("payload = % x, want % x", pdu.Payload, tt.wantBody)
			}
			if _, err := pdu.Marshal(); err != nil {
				t.Errorf("Marshal: %v", err)
			}
		})
	}
}

// fakeNEIssueReporter records the LI-plane faults reported to the ADMF.
type fakeNEIssueReporter struct {
	issues []string
}

func (f *fakeNEIssueReporter) Notify(issueType, _ string) {
	f.issues = append(f.issues, issueType)
}

// NotifyAsync records synchronously; see recordingReporter.NotifyAsync for why the
// double does not reproduce the dispatch.
func (f *fakeNEIssueReporter) NotifyAsync(issueType, description string) {
	f.Notify(issueType, description)
}

// TestCheckTagReportsUnusableTag proves an unusable content tag is reported to the
// ADMF over X1 instead of silently shipping product the MDF cannot
// correlate. A tag whose metadata never reached the datapath encap carries a zero
// correlation or an unknown action — interception runs but its product is useless.
func TestCheckTagReportsUnusableTag(t *testing.T) {
	tests := []struct {
		name       string
		tag        []byte
		wantReport bool
		// wantShip is the half that matters to an agency: a reported copy may still
		// be deliverable, and one that is not must be dropped rather than labelled
		// with a direction this element cannot read off the tag.
		wantShip bool
	}{
		{
			name:       "valid uplink tag is not reported",
			tag:        []byte{1, 0, 0, 0, 0, 0, 0, 0, farForwardUAndDuplicate},
			wantReport: false,
			wantShip:   true,
		},
		{
			name:       "valid encapsulated uplink tag is not reported",
			tag:        []byte{1, 0, 0, 0, 0, 0, 0, 0, farForwardUEncapAndDuplicate},
			wantReport: false,
			wantShip:   true,
		},
		{
			name:       "unknown action is reported and dropped",
			tag:        []byte{1, 0, 0, 0, 0, 0, 0, 0, 16},
			wantReport: true,
			// The action byte selects the payload format and direction, so an
			// unreadable one leaves no correct label. Delivering it anyway asserts a
			// fact about the subject's traffic the element does not know, and arrives
			// indistinguishable from a correct copy — worse than the gap a drop leaves,
			// which the mediation function can at least see.
			wantShip: false,
		},
		{
			name:       "zero correlation is reported but still delivered",
			tag:        []byte{0, 0, 0, 0, 0, 0, 0, 0, farForwardUAndDuplicate},
			wantReport: true,
			// The warrant identifier still attributes this copy, so an agency can hold
			// it; only the join to that session's signalling is lost.
			wantShip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeNEIssueReporter{}
			s := &liShipper{reporter: fake}

			if got := s.checkTag(append(tt.tag, 0x45, 0x00)); got != tt.wantShip {
				t.Errorf("checkTag ship = %v, want %v", got, tt.wantShip)
			}

			if got := len(fake.issues) > 0; got != tt.wantReport {
				t.Errorf("reported = %v (%v), want %v", got, fake.issues, tt.wantReport)
			}
			for _, issue := range fake.issues {
				if issue != x1.NEIssueX3TagInvalid {
					t.Errorf("issue type = %q, want %q", issue, x1.NEIssueX3TagInvalid)
				}
			}
		})
	}
}

// stubSender is a delivery client with an opinion about a destination it does not have, so
// the shipper's fault probes can be driven without an MDF3 to take away.
type stubSender struct{ down bool }

func (s *stubSender) Send(*x2x3.PDU) error { return nil }
func (s *stubSender) Close() error         { return nil }
func (s *stubSender) Unreachable() bool    { return s.down }

// triggerTo is an LI_T3 trigger delivering content to one MDF3, which is the only part of a
// trigger the delivery probe reads.
func triggerTo(xid, addr string) types.InterceptTask {
	return types.InterceptTask{
		XID:        types.XID(xid),
		Products:   []types.ProductType{types.ProductCC},
		Deliveries: []types.DeliveryEndpoint{{Type: types.DeliveryX3, Address: addr}},
	}
}

// TestShipperDeliveryProbeAnswersOnBothEdges covers what this element tells a triggering
// function that asks how it is, and the two quiet assertions matter as much as the loud one.
//
// A probe stuck *off* leaves a CC-POI that is delivering nothing answering that it is fine —
// which nothing downstream can contradict, since a copy that was never delivered produces no
// record for anybody to miss. A probe stuck *on* makes every healthy element report itself
// faulty, which is how this library's predecessor probe failed.
func TestShipperDeliveryProbeAnswersOnBothEdges(t *testing.T) {
	s := &liShipper{senders: make(map[string]x2x3.Sender), tasks: store.New()}
	probe := s.faultProbes()[0]

	if fault := probe(); fault != nil {
		t.Errorf("a shipper that has delivered nothing reports a delivery fault: %q",
			fault.ErrorDescription)
	}

	// Two agencies' triggers, each naming its own MDF3, as the CC-TF installs them.
	s.tasks.Activate(triggerTo("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa", "10.0.60.122:42069"))
	s.tasks.Activate(triggerTo("bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", "10.0.60.123:42069"))
	failing := &stubSender{down: true}
	s.senders["10.0.60.122:42069"] = failing
	s.senders["10.0.60.123:42069"] = &stubSender{}

	fault := probe()
	if fault == nil {
		t.Fatal("with an MDF3 unreachable the element reports no fault; content is being " +
			"dropped and nothing downstream can notice")
	}
	if !strings.Contains(fault.ErrorDescription, x1.NEIssueMDFUnreachable) {
		t.Errorf("the fault does not name the condition: %q", fault.ErrorDescription)
	}
	if !strings.Contains(fault.ErrorDescription, "1 of 2") {
		t.Errorf("the fault does not say how much is wrong: %q", fault.ErrorDescription)
	}
	// One agency's destination failing must not name that agency: TS 103 221-1 keeps an
	// element's own status separate from the per-destination faults reported per DID.
	for _, identity := range []string{"10.0.60.122", "42069"} {
		if strings.Contains(fault.ErrorDescription, identity) {
			t.Errorf("the element's own status names %q; it must say how much is wrong, never whose",
				identity)
		}
	}

	// Nothing clears it: delivery starts working and the next answer says so.
	failing.down = false
	if fault := probe(); fault != nil {
		t.Errorf("the fault outlived the condition, with nothing having cleared it: %q",
			fault.ErrorDescription)
	}
}

// TestLostCopiesAreNotAnElementStatus holds the loss counters as events, not as element
// status.
//
// "Am I losing copies" is tempting to answer from the reports this element already sends,
// but loss in the last N seconds is retention with an expiry — it discards real faults on a
// timer nobody can justify, and without one the element is faulty forever after a single
// burst. What is a state is the condition underneath, which the egress probe answers.
func TestLostCopiesAreNotAnElementStatus(t *testing.T) {
	fake := &fakeNEIssueReporter{}
	s := &liShipper{reporter: fake, senders: make(map[string]x2x3.Sender)}

	s.report(x1.NEIssueX3PuntLost, "content copies discarded at the datapath egress socket")
	s.report(x1.NEIssueX3FramingLost, "content copies dropped before framing")
	s.report(x1.NEIssueX3DeliveryLost, "content copies dropped from the delivery queue")

	if len(fake.issues) != 3 {
		t.Fatalf("lost copies reported %d times, want 3; the push reporting is what carries them",
			len(fake.issues))
	}
	for i, probe := range s.faultProbes() {
		if fault := probe(); fault != nil {
			t.Errorf("probe %d reports a fault after a burst of loss that has ended: %q; "+
				"an element that stays faulty on a past event is one nobody asks again",
				i, fault.ErrorDescription)
		}
	}
}

// TestEgressProbeFollowsTheSocket drives the real reconnect path, because the state and the
// answer have to agree and it is the reconnect that owns the state.
//
// The datapath is away when the loop starts redialling and comes back while it is still
// trying, which is what a BESS restart looks like from here.
func TestEgressProbeFollowsTheSocket(t *testing.T) {
	// The X3 egress is an AF_UNIX SEQPACKET socket, which not every development platform
	// provides; the datapath only ever runs on one that does.
	addr := filepath.Join(t.TempDir(), "li-x3.sock")
	probeLn, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Skipf("unixpacket sockets unavailable here: %v", err)
	}
	if closeErr := probeLn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	// A connection to close, standing in for the one the read failed on. Nothing is
	// listening on addr yet, so the redial inside reconnect cannot succeed.
	sock, _ := net.Pipe()
	s := &liShipper{sockAddr: addr, sock: sock, senders: make(map[string]x2x3.Sender)}
	egress := s.faultProbes()[1]

	if fault := egress(); fault != nil {
		t.Errorf("a shipper with a live egress socket reports it down: %q", fault.ErrorDescription)
	}

	reconnected := make(chan struct{})
	go func() {
		s.reconnect()
		close(reconnected)
	}()

	waitUntil(t, func() bool { return egress() != nil },
		"the egress socket went away and the element still answers that nothing is wrong")
	if fault := egress(); !strings.Contains(fault.ErrorDescription, x1.NEIssueX3EgressDown) {
		t.Errorf("the fault does not name the condition: %q", fault.ErrorDescription)
	}

	// The datapath comes back. Nothing clears the fault: the socket reconnects and the next
	// answer reflects that.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Fatalf("re-listening on the egress socket: %v", err)
	}
	defer ln.Close()

	select {
	case <-reconnected:
	case <-time.After(10 * time.Second):
		t.Fatal("the shipper never reconnected to the egress socket")
	}
	if fault := egress(); fault != nil {
		t.Errorf("the fault outlived the outage: %q", fault.ErrorDescription)
	}
}

// waitUntil polls cond until it holds, failing with why if it never does. The states these
// probes report are set by another goroutine, so an assertion has to be allowed to wait —
// but only for as long as the thing it is waiting for could legitimately take.
func waitUntil(t *testing.T, cond func() bool, why string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal(why)
}

// TestTheTwoShipperFaultsAreIndependent: an unreachable mediation function and a dead egress
// socket are different faults with different responses — one is outside this element and one
// is inside it — so neither may stand for or mask the other.
//
// It also covers the egress probe on a platform without SEQPACKET sockets, where the
// reconnect test above can only skip.
func TestTheTwoShipperFaultsAreIndependent(t *testing.T) {
	s := &liShipper{
		senders: map[string]x2x3.Sender{"10.0.60.122:42069": &stubSender{down: true}},
		tasks:   store.New(),
	}
	s.tasks.Activate(triggerTo("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa", "10.0.60.122:42069"))
	s.egressDown.Store(true)

	faults := make([]string, 0, 2)
	for _, probe := range s.faultProbes() {
		fault := probe()
		if fault == nil {
			t.Fatal("a probe stayed quiet with both conditions holding; each fault must be " +
				"reported on its own")
		}
		faults = append(faults, fault.ErrorDescription)
	}

	if !strings.Contains(faults[0], x1.NEIssueMDFUnreachable) {
		t.Errorf("the delivery fault is not reported as %s: %q", x1.NEIssueMDFUnreachable, faults[0])
	}
	if !strings.Contains(faults[1], x1.NEIssueX3EgressDown) {
		t.Errorf("the egress fault is not reported as %s: %q", x1.NEIssueX3EgressDown, faults[1])
	}

	// Each alone is reported alone: an element with a live egress and a failing MDF3 must not
	// be read as a datapath problem, and vice versa.
	s.egressDown.Store(false)
	if fault := s.faultProbes()[1](); fault != nil {
		t.Errorf("the egress is reported down while only delivery is failing: %q", fault.ErrorDescription)
	}
}

// TestAWithdrawnTriggersDestinationStopsCounting is the probe-stuck-on failure this element
// is most exposed to, because its triggers are withdrawn as sessions end — several times an
// hour on a busy UPF.
//
// A destination whose last delivery failed and whose trigger has since gone can never be
// delivered to again. Nothing would clear it, so counting it would leave the element
// reporting itself faulty for the life of the process, including while holding no tasking at
// all — an element that answers "Faults" to everything is ignored exactly as fast as one
// that answers "OK" to everything.
func TestAWithdrawnTriggersDestinationStopsCounting(t *testing.T) {
	const (
		xid  = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
		addr = "10.0.60.122:42069"
	)
	s := &liShipper{
		senders: map[string]x2x3.Sender{addr: &stubSender{down: true}},
		tasks:   store.New(),
	}
	s.tasks.Activate(triggerTo(xid, addr))
	probe := s.faultProbes()[0]

	if fault := probe(); fault == nil {
		t.Fatal("an unreachable MDF3 under a live trigger reported no fault")
	}

	// The warrant ends. The client stays — nothing removes it — but it is no longer a
	// destination this element delivers to.
	s.tasks.Deactivate(types.XID(xid))

	if fault := probe(); fault != nil {
		t.Errorf("a destination no trigger names is still reported as a fault: %q; nothing can "+
			"ever deliver there again, so nothing could clear it", fault.ErrorDescription)
	}
}

// capturingSender records what was delivered to one destination, so a fan-out can be
// asserted per destination rather than in aggregate.
//
// It keeps the wire form as well as the pointer. A shared PDU means every destination
// holds the same pointer, so comparing pointers says nothing about whether one sender
// altered the value before the next saw it — and the alteration to fear is specific:
// a sender stamping its own per-connection sequence number, which is exactly what
// TS 103 221-2 clause 5.3.9 says the number must not be. Marshalling at the moment of
// delivery captures what that destination's mediation function would receive.
type capturingSender struct {
	mu    sync.Mutex
	pdus  []*x2x3.PDU
	bytes [][]byte
}

func (c *capturingSender) Send(pdu *x2x3.PDU) error {
	b, err := pdu.Marshal()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pdus = append(c.pdus, pdu)
	c.bytes = append(c.bytes, b)

	return nil
}

func (c *capturingSender) Close() error { return nil }

func (c *capturingSender) delivered() []*x2x3.PDU {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]*x2x3.PDU(nil), c.pdus...)
}

// onTheWire is what each delivery would have put on its connection.
func (c *capturingSender) onTheWire() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([][]byte(nil), c.bytes...)
}

// multiDestinationTrigger is an LI_T3 trigger naming several MDF3s, which the
// provisioning interface permits and RequireResolvableDIDs validates. The criterion
// is the PFCP session identity, so the shipper finds the task without a duplication
// control behind it.
func multiDestinationTrigger(seid uint64, addrs ...string) types.InterceptTask {
	task := types.InterceptTask{
		XID:           "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		ProductID:     testXIDSEIDShaped,
		CorrelationID: 0x2632898145f4d191,
		Products:      []types.ProductType{types.ProductCC},
		Targets: []types.TargetIdentifier{
			{Type: types.TargetFSEID, Value: strconv.FormatUint(seid, 10)},
		},
	}
	for _, addr := range addrs {
		task.Deliveries = append(task.Deliveries,
			types.DeliveryEndpoint{Type: types.DeliveryX3, Address: addr})
	}

	return task
}

// TestShipDeliversToEveryX3Destination is the loss this fixes: a task resolving two
// X3 DIDs delivered to whichever endpoint happened to be first, and the agency
// behind the other received an empty stream while the element reported the
// interception as running.
//
// It also pins the two properties that make the fan-out a delivery and not a loop.
// One xCC carries one sequence number wherever it goes — the number belongs to the
// (XID, Correlation Identifier) context, not to a connection (TS 103 221-2 clause
// 5.3.9), and numbering per destination would give each mediation function gaps it
// could not tell from loss. And the PDU is built once, so a second destination costs
// its own delivery and not a second framing.
func TestShipDeliversToEveryX3Destination(t *testing.T) {
	const (
		first  = "10.0.60.122:42069"
		second = "10.0.60.123:42069"
		seid   = 4242
	)
	a, b := &capturingSender{}, &capturingSender{}
	s := &liShipper{
		tasks:   store.New(),
		senders: map[string]x2x3.Sender{first: a, second: b},
		ids:     x2x3.NewIdentity(testNEID, upfInterceptionPoint),
	}
	s.tasks.Activate(multiDestinationTrigger(seid, first, second))

	const copies = 4
	for range copies {
		s.ship(taggedCopy(seid, farForwardUAndDuplicate, uplinkCopy(443)))
	}

	atFirst, atSecond := a.delivered(), b.delivered()
	if len(atFirst) != copies || len(atSecond) != copies {
		t.Fatalf("delivered %d and %d copies, want %d to each destination",
			len(atFirst), len(atSecond), copies)
	}

	wireFirst, wireSecond := a.onTheWire(), b.onTheWire()

	for i := range atFirst {
		// The same pointer at both, which is what "built once and shared" means: a
		// loop calling shipperPDU per destination would hand out two values.
		if atFirst[i] != atSecond[i] {
			t.Errorf("copy %d was framed separately per destination; the framing must be shared", i)
		}
		// And byte-identical on both connections, which is the property sharing is
		// *for*. A sender that altered the shared value — stamping its own sequence
		// number, say — would leave the pointers equal and these bytes different.
		if !bytes.Equal(wireFirst[i], wireSecond[i]) {
			t.Errorf("copy %d reached the two destinations as different bytes; a sender mutated the shared PDU", i)
		}
		n := x3AttrsOf(atFirst[i])[x2x3.AttrSequenceNumber]
		m := x3AttrsOf(atSecond[i])[x2x3.AttrSequenceNumber]
		if !bytes.Equal(n, m) {
			t.Errorf("copy %d numbered % x at one destination and % x at the other", i, n, m)
		}
		if seq := binary.BigEndian.Uint32(n); seq != uint32(i) {
			t.Errorf("copy %d carries sequence number %d; the stream is not contiguous", i, seq)
		}
	}
}

// nullSender accepts delivery and keeps nothing, so an allocation measurement of
// the shipper is not measuring the recorder.
type nullSender struct{}

func (nullSender) Send(*x2x3.PDU) error { return nil }
func (nullSender) Close() error         { return nil }

// TestSecondDestinationCostsOnlyItsDelivery is the risk this fan-out carries, held
// to a number rather than argued. Content interception is the one path whose work is
// per packet, so a second destination must cost its own delivery and not a second
// framing — which is what building the PDU once buys, and what a loop calling
// shipperPDU per destination would have thrown away.
func TestSecondDestinationCostsOnlyItsDelivery(t *testing.T) {
	if raceEnabled {
		// The detector allocates on its own account, and this assertion is a
		// one-allocation margin: it measures instrumentation rather than framing once
		// -race is on. Skipped rather than loosened, because a bound wide enough to
		// survive the detector would no longer catch a second framing.
		t.Skip("allocation counts include the race detector's own allocations")
	}

	const (
		first  = "10.0.60.122:42069"
		second = "10.0.60.123:42069"
		seid   = 4242
	)
	shipperWith := func(addrs ...string) *liShipper {
		s := &liShipper{
			tasks:   store.New(),
			senders: map[string]x2x3.Sender{first: nullSender{}, second: nullSender{}},
			ids:     x2x3.NewIdentity(testNEID, upfInterceptionPoint),
		}
		s.tasks.Activate(multiDestinationTrigger(seid, addrs...))

		return s
	}

	toOne, toBoth := shipperWith(first), shipperWith(first, second)
	copied := taggedCopy(seid, farForwardUAndDuplicate, uplinkCopy(443))
	one := testing.AllocsPerRun(500, func() { toOne.ship(copied) })
	two := testing.AllocsPerRun(500, func() { toBoth.ship(copied) })

	t.Logf("allocations per send: one destination %.2f, two %.2f", one, two)
	if two > one {
		t.Errorf("a two-destination send allocates %.0f against %.0f for one; the framing "+
			"is being repeated per destination", two, one)
	}
}

// TestShipDeliversDespiteOneUnreachableDestination: one mediation function this
// element cannot deliver to must not deny the others a warrant's product. The
// element has no delivery credentials, so the destination with no client already
// made cannot get one — which is the condition, and the fault names which
// destination it is.
func TestShipDeliversDespiteOneUnreachableDestination(t *testing.T) {
	const (
		reachable = "10.0.60.122:42069"
		refused   = "10.0.60.123:42069"
		seid      = 100
	)
	rec := &recordingReporter{}
	a := &capturingSender{}
	s := &liShipper{
		tasks:    store.New(),
		senders:  map[string]x2x3.Sender{reachable: a},
		ids:      x2x3.NewIdentity(testNEID, upfInterceptionPoint),
		reporter: rec,
	}
	// The unreachable one is named first, so a fan-out that stopped at the first
	// failure would deliver nothing at all.
	s.tasks.Activate(multiDestinationTrigger(seid, refused, reachable))

	s.ship(taggedCopy(seid, farForwardUAndDuplicate, uplinkCopy(443)))

	if n := len(a.delivered()); n != 1 {
		t.Fatalf("the reachable destination received %d copies, want 1 — one unreachable "+
			"mediation function denied another its product", n)
	}
	if got := rec.reported(); len(got) != 1 || got[0] != x1.NEIssueMDFUnreachable {
		t.Fatalf("reported %v, want one %q", got, x1.NEIssueMDFUnreachable)
	}
	if d := rec.described()[0]; !strings.Contains(d, refused) {
		t.Errorf("the fault does not say which destination failed: %q", d)
	}
}

// x3AttrsOf indexes a PDU's conditional attributes by type, so an assertion can name
// the attribute it is about rather than a position.
func x3AttrsOf(pdu *x2x3.PDU) map[uint16][]byte {
	out := make(map[uint16][]byte, len(pdu.Attributes))
	for _, a := range pdu.Attributes {
		out[a.Type] = a.Value
	}

	return out
}

// TestShipperPDUCarriesTheRequiredAttributes covers the four conditional attributes
// TS 33.128 table 5.3.3-2 requires of an xCC, and the two it does not.
//
// li/x2x3 implements the mechanism but populates nothing itself, and an xCC carrying
// none of them is invisible to interoperability testing when neither side sends one,
// so the shipper's own PDU is asserted here.
func TestShipperPDUCarriesTheRequiredAttributes(t *testing.T) {
	task := types.InterceptTask{
		ProductID:     testXIDSEIDShaped,
		CorrelationID: 0x2632898145f4d191,
	}
	inner := []byte{0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	tagged := append(append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, farForwardUAndDuplicate), inner...)

	before := time.Now()
	pdu := shipperPDU(tagged, task, x2x3.NewIdentity(testNEID, upfInterceptionPoint))
	after := time.Now()

	attrs := x3AttrsOf(pdu)
	if len(attrs) != 4 {
		t.Errorf("PDU carries %d attributes, want the 4 of table 5.3.3-2: %+v", len(attrs), pdu.Attributes)
	}

	// The NFID is the identifier this element asserts on X1, so a mediation function
	// and the ADMF name the same element.
	if got := string(attrs[x2x3.AttrNFID]); got != testNEID {
		t.Errorf("NFID = %q, want the configured network element identifier %q", got, testNEID)
	}
	if got := string(attrs[x2x3.AttrIPID]); got != upfInterceptionPoint {
		t.Errorf("IPID = %q, want %q", got, upfInterceptionPoint)
	}

	// Table 5.3.3-2 requires neither target identifier, and this POI holds no
	// subscriber identity: it is tasked by packet-detection criteria. Sending one
	// would mean inventing it.
	for _, typ := range []uint16{x2x3.AttrMatchedTargetIdentifier, x2x3.AttrOtherTargetIdentifier} {
		if v, ok := attrs[typ]; ok {
			t.Errorf("attribute %d present with %q; X3 owes no target identity", typ, v)
		}
	}

	// The timestamp is when the xCC was generated (table 5.3.3-2), which is framing
	// time — so the datapath carries no timestamp and the tag does not grow.
	ts, ok := attrs[x2x3.AttrTimestamp]
	if !ok || len(ts) != 8 {
		t.Fatalf("timestamp attribute = % x, want 8 octets", ts)
	}
	when := time.Unix(int64(binary.BigEndian.Uint32(ts[0:4])), int64(binary.BigEndian.Uint32(ts[4:8])))
	if when.Before(before.Truncate(time.Second)) || when.After(after.Add(time.Second)) {
		t.Errorf("timestamp = %v, want between %v and %v", when, before, after)
	}
	if liTagLen != fseidTagLen+1 {
		t.Errorf("the datapath tag is %d bytes: a timestamp was added to it, which table 5.3.3-2 does not ask for", liTagLen)
	}
	if !bytes.Equal(pdu.Payload, inner) {
		t.Errorf("payload = % x, want the whole inner packet % x", pdu.Payload, inner)
	}

	if _, err := pdu.Marshal(); err != nil {
		t.Errorf("Marshal: %v", err)
	}
}

// TestShipperPDUNumbersEachContextIndependently is clause 5.3.9's context rule on the
// content path: two sessions of one warrant are two contexts, each numbered from
// zero. A counter held per connection or per element would interleave them, and a
// mediation function ordering one session's content would see gaps that are not loss.
func TestShipperPDUNumbersEachContextIndependently(t *testing.T) {
	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	inner := []byte{0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	tagged := append(append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, farForwardUAndDuplicate), inner...)

	sessionA := types.InterceptTask{ProductID: testXIDSEIDShaped, CorrelationID: 1}
	sessionB := types.InterceptTask{ProductID: testXIDSEIDShaped, CorrelationID: 2}

	seqOf := func(task types.InterceptTask) uint32 {
		return binary.BigEndian.Uint32(x3AttrsOf(shipperPDU(tagged, task, ids))[x2x3.AttrSequenceNumber])
	}

	for want := range uint32(3) {
		if got := seqOf(sessionA); got != want {
			t.Errorf("session A packet %d numbered %d", want, got)
		}
	}
	if got := seqOf(sessionB); got != 0 {
		t.Errorf("a second session starts at %d, want 0 — the counter is not per context", got)
	}
	if got := seqOf(sessionA); got != 3 {
		t.Errorf("session A continued at %d, want 3", got)
	}
}

// TestShipperPDUNumbersUnderConcurrentFraming: framing runs on liFrameWorkers
// goroutines, so the number is taken concurrently for one session. Every number must
// be handed out exactly once — a repeat would make two packets indistinguishable to a
// mediation function ordering them. Run under -race.
func TestShipperPDUNumbersUnderConcurrentFraming(t *testing.T) {
	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	task := types.InterceptTask{ProductID: testXIDSEIDShaped, CorrelationID: 7}
	inner := []byte{0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}

	const perWorker = 100
	var mu sync.Mutex
	seen := make(map[uint32]int, liFrameWorkers*perWorker)

	var wg sync.WaitGroup
	for range liFrameWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				tagged := append(append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, farForwardUAndDuplicate), inner...)
				n := binary.BigEndian.Uint32(x3AttrsOf(shipperPDU(tagged, task, ids))[x2x3.AttrSequenceNumber])
				mu.Lock()
				seen[n]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != liFrameWorkers*perWorker {
		t.Errorf("%d distinct numbers for %d packets: a sequence number was reused", len(seen), liFrameWorkers*perWorker)
	}
}

// TestShipperRefusesWithoutAnElementIdentifier: content
// that a mediation function cannot attribute to the element that produced it is not
// delivered at all. The datapath is unaffected — it keeps forwarding — because a UPF
// that refuses to come up over its LI configuration tells every operator it is
// LI-provisioned.
func TestShipperRefusesWithoutAnElementIdentifier(t *testing.T) {
	if err := startLIShipper(&LiConfig{X3SockAddr: "/nonexistent.sock"}, nil, nil); !errors.Is(err, errNoElementIdentifier) {
		t.Errorf("startLIShipper without a network element identifier returned %v, want errNoElementIdentifier", err)
	}
}

// triggerToDID is triggerTo with the destination identifier the ADMF provisioned
// the endpoint under, which is what a destination-scoped fault report names.
func triggerToDID(xid, addr, did string) types.InterceptTask {
	t := triggerTo(xid, addr)
	t.Deliveries[0].DID = did

	return t
}

// TestTheWatcherSeesWhatTheRetiredSitesReported guards the most likely way routing these
// reports through the watcher could make something worse.
//
// Two sites notice an unreachable MDF3 first — the delivery-failure hook and the
// keepalive one — and both nudge the watcher rather than reporting. If the watcher's own
// view does not cover what they saw, that fault is reported by nobody.
func TestTheWatcherSeesWhatTheRetiredSitesReported(t *testing.T) {
	s := &liShipper{senders: make(map[string]x2x3.Sender), tasks: store.New()}

	const (
		failingAddr = "10.0.60.122:42069"
		healthyAddr = "10.0.60.123:42069"
		failingDID  = "aaaaaaaa-3333-4333-8333-aaaaaaaaaaaa"
		healthyDID  = "bbbbbbbb-4444-4444-8444-bbbbbbbbbbbb"
	)

	s.tasks.Activate(triggerToDID("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa", failingAddr, failingDID))
	s.tasks.Activate(triggerToDID("bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", healthyAddr, healthyDID))
	s.senders[failingAddr] = &stubSender{down: true}
	s.senders[healthyAddr] = &stubSender{}

	health := s.destinationHealth()
	if len(health) != 2 {
		t.Fatalf("the watcher sees %d destinations, want 2: %+v", len(health), health)
	}

	for _, h := range health {
		switch h.DID {
		case failingDID:
			if !h.Unreachable {
				t.Error("the watcher does not see the destination the retired sites reported; that fault is now reported by nobody")
			}
		case healthyDID:
			if h.Unreachable {
				t.Error("the watcher reports a working destination as unreachable")
			}
		default:
			t.Errorf("the watcher named a destination nothing provisioned: %+v", h)
		}
	}
}

// TestAnUntriedDestinationIsNotReportedUnreachable keeps the watcher to the same
// rule the probe follows: an element that has delivered nothing has not found a
// mediation function unreachable, it has not looked. Without it, every element
// would report every destination faulty from startup until its first delivery.
func TestAnUntriedDestinationIsNotReportedUnreachable(t *testing.T) {
	s := &liShipper{senders: make(map[string]x2x3.Sender), tasks: store.New()}
	s.tasks.Activate(triggerToDID("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		"10.0.60.122:42069", "aaaaaaaa-3333-4333-8333-aaaaaaaaaaaa"))

	for _, h := range s.destinationHealth() {
		if h.Unreachable {
			t.Errorf("a destination nothing has been sent to is reported unreachable: %+v", h)
		}
	}
}

// TestShipperStartsBeforeTheDatapathDoes is the initialisation race the two
// containers make ordinary rather than exotic.
//
// `bess` and `pfcp-agent` are ordinary containers in one pod, started concurrently
// with no readiness gate between them, so the datapath's egress socket may simply
// not exist when the agent reaches it. A single dial would fail, the CC-POI would
// be absent for the life of the process, and the only trace would be a deliberately
// vague line on the BESS log.
//
// What must hold instead: the shipping loop keeps trying, and everything that does
// not depend on the datapath is up in the meantime — the element can be tasked and
// can answer that its egress is down, which is a state a triggering function can
// act on.
func TestShipperStartsBeforeTheDatapathDoes(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "li-x3.sock")

	// Confirm the platform has SEQPACKET at all before concluding anything from a
	// failure to dial one.
	probeLn, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Skipf("unixpacket sockets unavailable here: %v", err)
	}
	if closeErr := probeLn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	s := &liShipper{sockAddr: addr, senders: make(map[string]x2x3.Sender)}
	s.egressDown.Store(true)
	egress := s.faultProbes()[1]

	// Nothing is listening yet, which is the whole point.
	if fault := egress(); fault == nil {
		t.Fatal("a shipper whose datapath has not arrived answers that nothing is wrong")
	}

	established := make(chan struct{})
	go func() {
		s.establish()
		close(established)
	}()

	// Still down while it retries, rather than having given up.
	select {
	case <-established:
		t.Fatal("the shipper established an egress nothing was listening on")
	case <-time.After(50 * time.Millisecond):
	}
	if fault := egress(); !strings.Contains(fault.ErrorDescription, x1.NEIssueX3EgressDown) {
		t.Errorf("the fault does not name the condition: %q", fault.ErrorDescription)
	}

	// The datapath arrives.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Fatalf("listening on the egress socket: %v", err)
	}
	defer ln.Close()

	select {
	case <-established:
	case <-time.After(10 * time.Second):
		t.Fatal("the shipper never established the egress after the datapath arrived")
	}
	if fault := egress(); fault != nil {
		t.Errorf("the egress is up and the element still reports it down: %q", fault.ErrorDescription)
	}
}

// TestEstablishDoesNotSleepBeforeItsFirstAttempt keeps the initial dial from paying
// the backoff a *redial* is owed. reconnect sleeps first on purpose — a socket that
// just failed should not be hammered — and establish, carved out of it, must not
// inherit that for the case where the datapath is already up.
func TestEstablishDoesNotSleepBeforeItsFirstAttempt(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "li-x3.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unixpacket", addr)
	if err != nil {
		t.Skipf("unixpacket sockets unavailable here: %v", err)
	}
	defer ln.Close()

	s := &liShipper{sockAddr: addr, senders: make(map[string]x2x3.Sender)}
	s.egressDown.Store(true)

	start := time.Now()
	s.establish()

	if took := time.Since(start); took >= minReconnectDelay {
		t.Errorf("establish took %s against a datapath that was already up; "+
			"the first attempt slept before trying", took)
	}
	if s.egressDown.Load() {
		t.Error("the egress is established and the element still reports it down")
	}
	if s.sock != nil {
		_ = s.sock.Close()
	}
}
