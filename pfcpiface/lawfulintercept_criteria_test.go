// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/hex"
	"math"
	"net"
	"testing"

	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/wmnsk/go-pfcp/ie"
)

// The sessions these tests resolve against, chosen so that each criterion has
// something it must select and something it must not:
//
//	seid 100 — the target: UE 10.250.0.9, uplink TEID 0x1001 on 10.76.0.2, DNN
//	           "internet". Its two PDRs reference separate FARs, so a criterion
//	           selecting one direction covers exactly that direction.
//	seid 200 — another subscriber on the same DNN, so a network-instance criterion
//	           must select it too and an address criterion must not.
//	seid 300 — a session whose uplink and downlink PDRs share one FAR, which is how
//	           duplication comes to over-collect: enabling it on that FAR copies the
//	           direction the criterion did not select.
const (
	targetSEID = uint64(100)
	otherSEID  = uint64(200)
	sharedSEID = uint64(300)
)

func testNetInstance() string { return "internet" }

func criteriaSessions() []PFCPSession {
	ue := ip2int(net.ParseIP(testUEIPv4))
	n3 := ip2int(net.ParseIP("10.76.0.2"))

	return []PFCPSession{
		{
			localSEID: targetSEID,
			PacketForwardingRules: PacketForwardingRules{
				pdrs: []pdr{
					uplinkPDR(targetSEID, 1, 0x1001, n3, ue),
					downlinkPDR(targetSEID, 2, ue),
				},
				fars: []far{{farID: 1, fseID: targetSEID}, {farID: 2, fseID: targetSEID}},
			},
		},
		{
			localSEID: otherSEID,
			PacketForwardingRules: PacketForwardingRules{
				pdrs: []pdr{
					uplinkPDR(otherSEID, 1, 0x2001, n3, ip2int(net.ParseIP("10.250.0.10"))),
				},
				fars: []far{{farID: 1, fseID: otherSEID}},
			},
		},
		{
			localSEID: sharedSEID,
			PacketForwardingRules: PacketForwardingRules{
				pdrs: []pdr{
					uplinkPDR(sharedSEID, 9, 0x3001, n3, ip2int(net.ParseIP("10.250.0.11"))),
					downlinkPDR(sharedSEID, 9, ip2int(net.ParseIP("10.250.0.11"))),
				},
				fars: []far{{farID: 9, fseID: sharedSEID}},
			},
		},
	}
}

// uplinkPDR builds the uplink rule of a session. Its PDR ID is always 1: the
// downlink rule takes 2, matching how the SMF numbers them.
func uplinkPDR(seid uint64, farID, teid, n3, ue uint32) pdr {
	return pdr{
		pdrID: 1, fseID: seid, farID: farID,
		srcIface: access, srcIfaceMask: 0xff,
		tunnelTEID: teid, tunnelTEIDMask: math.MaxUint32,
		tunnelIP4Dst: n3, tunnelIP4DstMask: math.MaxUint32,
		ueAddress:       ue,
		qerIDList:       []uint32{4},
		networkInstance: testNetInstance(),
		appFilter: applicationFilter{
			srcIP: ue, srcIPMask: math.MaxUint32,
			srcPortRange: newWildcardPortRange(), dstPortRange: newWildcardPortRange(),
		},
	}
}

// downlinkPDR builds the downlink rule of a session, PDR ID 2 to the uplink's 1.
func downlinkPDR(seid uint64, farID, ue uint32) pdr {
	return pdr{
		pdrID: 2, fseID: seid, farID: farID,
		srcIface: core, srcIfaceMask: 0xff,
		ueAddress:       ue,
		qerIDList:       []uint32{4},
		networkInstance: testNetInstance(),
		appFilter: applicationFilter{
			dstIP: ue, dstIPMask: math.MaxUint32,
			srcPortRange: newWildcardPortRange(), dstPortRange: newWildcardPortRange(),
		},
	}
}

// sel is one expected selection: a PDR of a session, and how closely it
// corresponds to the criterion.
type sel struct {
	seid  uint64
	pdrID uint32
	cover coverage
}

// TestResolveCriteria checks which PDRs each detection criterion of TS 33.128
// table 6.2.3-7 selects across every live session, including the criteria that
// select nothing. Selecting the wrong PDRs is not a visible failure at runtime —
// it silently intercepts another subscriber's traffic, or none — so each case
// pins the exact set.
func TestResolveCriteria(t *testing.T) {
	sessions := criteriaSessions()
	niHex := hex.EncodeToString([]byte(testNetInstance()))

	cases := []struct {
		name string
		id   types.TargetIdentifier
		want []sel
	}{
		{
			name: "PFCP session ID selects both directions of one session",
			id:   types.TargetIdentifier{Type: types.TargetFSEID, Value: "100"},
			want: []sel{{targetSEID, 1, coverExact}, {targetSEID, 2, coverExact}},
		},
		{
			name: "PFCP session ID of no live session selects nothing",
			id:   types.TargetIdentifier{Type: types.TargetFSEID, Value: "999"},
			want: nil,
		},
		{
			name: "F-TEID selects the uplink PDR that matches on the tunnel",
			id:   types.TargetIdentifier{Type: types.TargetFTEID, Value: "4097"}, // 0x1001
			want: []sel{{targetSEID, 1, coverExact}},
		},
		{
			name: "F-TEID with the wrong node address selects nothing",
			id:   types.TargetIdentifier{Type: types.TargetFTEID, Value: "4097@10.76.0.99"},
			want: nil,
		},
		{
			name: "F-TEID with the right node address still selects it",
			id:   types.TargetIdentifier{Type: types.TargetFTEID, Value: "4097@10.76.0.2"},
			want: []sel{{targetSEID, 1, coverExact}},
		},
		{
			name: "UE address selects both directions, and only that subscriber",
			id:   types.TargetIdentifier{Type: types.TargetUEIPv4, Value: testUEIPv4},
			want: []sel{{targetSEID, 1, coverExact}, {targetSEID, 2, coverExact}},
		},
		{
			name: "an unattached UE address selects nothing",
			id:   types.TargetIdentifier{Type: types.TargetUEIPv4, Value: "10.250.0.99"},
			want: nil,
		},
		// No case here selects by PDR ID or QER ID. Those identifiers are allocated per
		// PFCP session and reused, so selecting by one alone would intercept subjects the
		// warrant never named; the refusal is asserted in TestParseCriterionRefusals.
		{
			name: "network instance spans every session on the DNN",
			id:   types.TargetIdentifier{Type: types.TargetNetworkInstance, Value: niHex},
			want: []sel{
				{targetSEID, 1, coverExact},
				{targetSEID, 2, coverExact},
				{otherSEID, 1, coverExact},
				{sharedSEID, 1, coverExact},
				{sharedSEID, 2, coverExact},
			},
		},
		{
			name: "another network instance selects nothing",
			id: types.TargetIdentifier{
				Type: types.TargetNetworkInstance, Value: hex.EncodeToString([]byte("ims")),
			},
			want: nil,
		},
		{
			name: "inbound direction selects the uplink PDRs only",
			id:   types.TargetIdentifier{Type: types.TargetGTPTunnelDirection, Value: x1.GTPDirectionInbound},
			want: []sel{{targetSEID, 1, coverExact}, {otherSEID, 1, coverExact}, {sharedSEID, 1, coverExact}},
		},
		{
			name: "outbound direction selects the downlink PDRs only",
			id:   types.TargetIdentifier{Type: types.TargetGTPTunnelDirection, Value: x1.GTPDirectionOutbound},
			want: []sel{{targetSEID, 2, coverExact}, {sharedSEID, 2, coverExact}},
		},
		{
			name: "a port on wildcard rules selects them as broader than the criterion",
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"},
			want: []sel{
				{targetSEID, 1, coverBroader},
				{targetSEID, 2, coverBroader},
				{otherSEID, 1, coverBroader},
				{sharedSEID, 1, coverBroader},
				{sharedSEID, 2, coverBroader},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cr, err := parseCriterion(c.id)
			if err != nil {
				t.Fatalf("parseCriterion(%+v): %v", c.id, err)
			}
			got := cr.resolve(sessions)
			if len(got) != len(c.want) {
				t.Fatalf("selected %v, want %v", got, c.want)
			}
			for i, w := range c.want {
				if got[i].seid != w.seid || got[i].pdrID != w.pdrID || got[i].cover != w.cover {
					t.Errorf("selection %d = %v, want pdr(seid=%d, id=%d, %s)",
						i, got[i], w.seid, w.pdrID, w.cover)
				}
			}
		})
	}
}

// TestResolvePortCriterionAgainstSDFFilter covers the port cases the wildcard
// sessions above cannot: a PDR whose SDF filter does constrain the port, and one
// whose filter constrains it to something else.
func TestResolvePortCriterionAgainstSDFFilter(t *testing.T) {
	ue := ip2int(net.ParseIP(testUEIPv4))
	withFilter := func(direction uint8, proto uint8, low, high uint16) pdr {
		p := pdr{
			pdrID: 1, fseID: targetSEID, farID: 1,
			srcIface: direction, srcIfaceMask: 0xff, ueAddress: ue,
			appFilter: applicationFilter{proto: proto, protoMask: 0xff},
		}
		r := portRange{low: low, high: high}
		if direction == access {
			p.appFilter.srcPortRange, p.appFilter.dstPortRange = r, newWildcardPortRange()
		} else {
			p.appFilter.srcPortRange, p.appFilter.dstPortRange = newWildcardPortRange(), r
		}

		return p
	}

	cases := []struct {
		name string
		p    pdr
		id   types.TargetIdentifier
		want coverage
	}{
		{
			// The UE is the source uplink, so the criterion's port is the source port.
			name: "uplink exact match on the UE's port",
			p:    withFilter(access, protoTCP, 443, 443),
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"},
			want: coverExact,
		},
		{
			name: "uplink filter on another port",
			p:    withFilter(access, protoTCP, 80, 80),
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"},
			want: coverNone,
		},
		{
			// The UE is the destination downlink, so it is the destination port that
			// describes the UE's port — reading the source port here would intercept
			// on the far end's port instead.
			name: "downlink exact match on the UE's port",
			p:    withFilter(core, protoTCP, 443, 443),
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"},
			want: coverExact,
		},
		{
			name: "a range containing the port is broader",
			p:    withFilter(access, protoTCP, 400, 500),
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "443"},
			want: coverBroader,
		},
		{
			name: "a UDP criterion does not match a TCP filter",
			p:    withFilter(access, protoTCP, 443, 443),
			id:   types.TargetIdentifier{Type: types.TargetUDPPort, Value: "443"},
			want: coverNone,
		},
		{
			name: "a UDP criterion matches a UDP filter",
			p:    withFilter(access, protoUDP, 2152, 2152),
			id:   types.TargetIdentifier{Type: types.TargetUDPPort, Value: "2152"},
			want: coverExact,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cr, err := parseCriterion(c.id)
			if err != nil {
				t.Fatalf("parseCriterion: %v", err)
			}
			if got := cr.matchPDR(c.p); got != c.want {
				t.Errorf("coverage = %s, want %s", got, c.want)
			}
		})
	}
}

// TestParseCriterionRefusals checks that the criteria this agent cannot resolve
// are refused here rather than resolving to an empty selection. The two are very
// different: an empty selection is a task waiting for a session to appear, while a
// criterion that can never be evaluated is an interception that will never produce
// anything, and the triggering function has to be told.
func TestParseCriterionRefusals(t *testing.T) {
	cases := []struct {
		name string
		id   types.TargetIdentifier
	}{
		{
			name: "UE IPv6, which this datapath has no state for",
			id:   types.TargetIdentifier{Type: types.TargetUEIPv6, Value: testUEIPv6},
		},
		{
			name: "an encoded PDR, which needs comparison semantics we do not have",
			id:   types.TargetIdentifier{Type: types.TargetPDR, Value: "0a01"},
		},
		// The two rule identifiers, and this is the refusal that matters most in this file.
		// They are allocated per PFCP session and reused from a low number, so matching one
		// selected the rule of that number in *every* session this element holds — the
		// traffic of every subscriber whose session happens to hold a PDR 2, duplicated,
		// attributed and delivered under that warrant, indistinguishable downstream from
		// the traffic it did name.
		//
		// A session cannot qualify them: a task's identifiers are alternatives, so naming
		// an F-SEID beside the rule ID widens the interception rather than narrowing it.
		// TS 33.128 table 6.2.3-7 lists both, so this is a declared gap and not a silent
		// one.
		{
			name: "a PDR ID, which is allocated per session and reused across sessions",
			id:   types.TargetIdentifier{Type: types.TargetPDRID, Value: "2"},
		},
		{
			name: "a QER ID, likewise",
			id:   types.TargetIdentifier{Type: types.TargetQERID, Value: "4"},
		},
		{
			name: "a subscriber identity, which the UPF holds none of",
			id:   types.TargetIdentifier{Type: types.TargetSUPI, Value: "262019876543210"},
		},
		{
			name: "a malformed session ID",
			id:   types.TargetIdentifier{Type: types.TargetFSEID, Value: "not-a-number"},
		},
		{
			name: "a malformed UE address",
			id:   types.TargetIdentifier{Type: types.TargetUEIPv4, Value: "10.250.0"},
		},
		{
			name: "an IPv6 literal in the IPv4 arm",
			id:   types.TargetIdentifier{Type: types.TargetUEIPv4, Value: testUEIPv6},
		},
		{
			name: "a port of zero",
			id:   types.TargetIdentifier{Type: types.TargetTCPPort, Value: "0"},
		},
		{
			name: "a port beyond the range",
			id:   types.TargetIdentifier{Type: types.TargetUDPPort, Value: "65536"},
		},
		{
			name: "an odd-length network instance, which is not hexBinary",
			id:   types.TargetIdentifier{Type: types.TargetNetworkInstance, Value: "abc"},
		},
		{
			name: "a direction outside the enumeration",
			id:   types.TargetIdentifier{Type: types.TargetGTPTunnelDirection, Value: "sideways"},
		},
		{
			name: "a tunnel address that is not an address",
			id:   types.TargetIdentifier{Type: types.TargetFTEID, Value: "4097@nowhere"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseCriterion(c.id); err == nil {
				t.Errorf("parseCriterion(%+v) accepted a criterion it cannot resolve", c.id)
			}
		})
	}
}

// encodeCreatePDR builds the wire form of a Create PDR IE, which is what a PDR
// detection criterion carries. reorder swaps two IEs inside the PDI, producing a
// *different encoding of the same rule* — the case an octet comparison would miss.
func encodeCreatePDR(t *testing.T, teid uint32, ue string, reorder bool) string {
	t.Helper()
	pdi := []*ie.IE{
		ie.NewSourceInterface(ie.SrcInterfaceAccess),
		ie.NewFTEID(0x01, teid, net.ParseIP("10.76.0.2"), nil, 0),
		ie.NewUEIPAddress(0x02, ue, "", 0, 0),
	}
	if reorder {
		pdi[0], pdi[2] = pdi[2], pdi[0]
	}
	raw, err := ie.NewCreatePDR(
		ie.NewPDRID(1), ie.NewPrecedence(200), ie.NewPDI(pdi...),
		ie.NewFARID(1), ie.NewQERID(4),
	).Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	return hex.EncodeToString(raw)
}

// pdrCriterionSessions is one session whose uplink PDR is the rule the criteria
// below describe, plus another subscriber's, so a match on the wrong one shows up.
func pdrCriterionSessions(t *testing.T) []PFCPSession {
	t.Helper()
	sessions := criteriaSessions()
	// criteriaSessions builds its uplink PDRs through the same helpers, but with a
	// network instance and precedence that the encoded criterion does not carry, so
	// the rule to compare against is built here from the same IEs instead.
	rule, err := parsePDRCriterion(encodeCreatePDR(t, 0x1001, testUEIPv4, false))
	if err != nil {
		t.Fatalf("building the fixture rule: %v", err)
	}
	rule.fseID = targetSEID
	sessions[0].pdrs[0] = *rule

	return sessions
}

// TestPDRCriterionMatchesTheRuleItNames checks the criterion that needs a whole rule
// compared rather than a field. The comparison runs over this agent's parsed form,
// which is what makes it independent of how the rule was encoded — PFCP puts no
// ordering on the IEs inside a grouped IE, so an octet comparison would miss a
// legitimate match.
func TestPDRCriterionMatchesTheRuleItNames(t *testing.T) {
	sessions := pdrCriterionSessions(t)

	cases := []struct {
		name  string
		value string
		want  []sel
	}{
		{
			name:  "the rule the session holds",
			value: encodeCreatePDR(t, 0x1001, testUEIPv4, false),
			want:  []sel{{targetSEID, 1, coverExact}},
		},
		{
			// Same rule, different bytes. This is the case the criterion exists to
			// survive, and the reason the comparison is not octet-for-octet.
			name:  "the same rule encoded with its PDI in another order",
			value: encodeCreatePDR(t, 0x1001, testUEIPv4, true),
			want:  []sel{{targetSEID, 1, coverExact}},
		},
		{
			name:  "a rule for another tunnel selects nothing",
			value: encodeCreatePDR(t, 0x9999, testUEIPv4, false),
			want:  nil,
		},
		{
			name:  "a rule for another subscriber selects nothing",
			value: encodeCreatePDR(t, 0x1001, "10.250.0.10", false),
			want:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cr, err := parseCriterion(types.TargetIdentifier{Type: types.TargetPDR, Value: c.value})
			if err != nil {
				t.Fatalf("parseCriterion: %v", err)
			}
			got := cr.resolve(sessions)
			if len(got) != len(c.want) {
				t.Fatalf("selected %v, want %v", got, c.want)
			}
			for i, w := range c.want {
				if got[i].seid != w.seid || got[i].pdrID != w.pdrID || got[i].cover != w.cover {
					t.Errorf("selection %d = %v, want pdr(seid=%d, id=%d, %s)",
						i, got[i], w.seid, w.pdrID, w.cover)
				}
			}
		})
	}
}

// TestPDRCriterionIgnoresSessionAssignedFields checks that the fields a *session*
// assigns are excluded from the comparison. They are not properties of the rule the
// triggering function described, so including them would make a correct criterion
// match nothing — an interception that reports success and collects nothing.
func TestPDRCriterionIgnoresSessionAssignedFields(t *testing.T) {
	cr, err := parseCriterion(types.TargetIdentifier{
		Type: types.TargetPDR, Value: encodeCreatePDR(t, 0x1001, testUEIPv4, false),
	})
	if err != nil {
		t.Fatalf("parseCriterion: %v", err)
	}

	rule := *cr.rule
	// What a session does to a rule it installs: it belongs to a session, was sent
	// from an address, and is counted by a counter this UPF chose.
	rule.fseID = 0xdeadbeef
	rule.fseidIP = ip2int(net.ParseIP("10.76.0.5"))
	rule.ctrID = 77

	if cr.matchPDR(rule) != coverExact {
		t.Error("a rule differing only in what its session assigned did not match")
	}

	// But a difference in the rule itself must still tell them apart.
	rule.precedence++
	if cr.matchPDR(rule) != coverNone {
		t.Error("a rule with a different precedence matched")
	}
}

// TestPDRCriterionRefusals checks that a PDR criterion this agent cannot turn into a
// rule is refused rather than resolved to something that matches nothing — the two
// are indistinguishable afterwards, and the second is an acknowledged interception
// that can never produce anything.
func TestPDRCriterionRefusals(t *testing.T) {
	valid := encodeCreatePDR(t, 0x1001, testUEIPv4, false)

	// A Create FAR, so a well-formed PFCP element that is not a rule.
	far, err := ie.NewCreateFAR(ie.NewFARID(1), ie.NewApplyAction(0x02)).Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// A Create PDR whose UE IP Address IE asks the UPF to choose the address. That
	// describes no traffic, so it cannot be a criterion.
	alloc, err := ie.NewCreatePDR(
		ie.NewPDRID(1), ie.NewPrecedence(200),
		ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewUEIPAddress(0x10, "", "", 0, 0)),
		ie.NewFARID(1),
	).Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// An Update PDR carries the same fields as a Create PDR, so it parses perfectly
	// well as a rule. It is refused on the IE type alone: the criterion has to be one
	// agreed form, and Create PDR is the one the SMF sends.
	update, err := ie.NewUpdatePDR(
		ie.NewPDRID(1), ie.NewPrecedence(200),
		ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(0x01, 0x1001, net.ParseIP("10.76.0.2"), nil, 0),
			ie.NewUEIPAddress(0x02, testUEIPv4, "", 0, 0)),
		ie.NewFARID(1), ie.NewQERID(4),
	).Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	cases := []struct{ name, value string }{
		{
			name:  "not hex at all",
			value: testUnparseable,
		},
		{
			name:  "an Update PDR, which parses as a rule but is not the agreed form",
			value: hex.EncodeToString(update),
		},
		{
			name:  "odd-length hex, so not hexBinary",
			value: "0a1",
		},
		{
			name:  "empty",
			value: "",
		},
		{
			name:  "hex that is not a PFCP element",
			value: "ffffffffffffffff",
		},
		{
			name:  "a Create FAR rather than a Create PDR",
			value: hex.EncodeToString(far),
		},
		{
			name:  "a rule leaving the UE address to be allocated",
			value: hex.EncodeToString(alloc),
		},
		{
			name:  "a truncated Create PDR",
			value: valid[:len(valid)-8],
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseCriterion(types.TargetIdentifier{
				Type: types.TargetPDR, Value: c.value,
			}); err == nil {
				t.Error("parseCriterion accepted a PDR criterion it cannot resolve")
			}
		})
	}
}

// TestAPortCriterionDoesNotDeliverTheOtherProtocol is *coverage is a property of every
// dimension, not the first one checked*.
//
// A transport-port criterion constrains two things: the protocol and the port. If the protocol
// were used only to exclude — a rule pinning a different protocol matches nothing — and never
// to widen, a rule that pinned the port exactly and left the protocol unconstrained would
// return coverExact. Exact means the copy is delivered without inspection, so a TCP-port
// criterion would deliver the UDP traffic on that port too: traffic the warrant does not name,
// under the warrant's own identifier, indistinguishable downstream from the traffic it did.
func TestAPortCriterionDoesNotDeliverTheOtherProtocol(t *testing.T) {
	const port = 443

	for _, tc := range []struct {
		name      string
		protoMask uint8
		proto     uint8
		want      coverage
	}{
		{
			// The case the fix is about: the rule pins the port and not the protocol, so it
			// carries the UDP traffic on that port as well as the TCP.
			name: "the rule pins the port and not the protocol", protoMask: 0,
			want: coverBroader,
		},
		{
			// Both dimensions pinned: the rule carries exactly what the criterion names.
			name: "the rule pins both", protoMask: 0xff, proto: protoTCP,
			want: coverExact,
		},
		{
			name: "the rule pins the other protocol", protoMask: 0xff, proto: protoUDP,
			want: coverNone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseCriterion(types.TargetIdentifier{
				Type: types.TargetTCPPort, Value: "443",
			})
			if err != nil {
				t.Fatalf("parseCriterion: %v", err)
			}

			// A downlink rule, so the criterion's port is compared against the destination
			// range — the direction the appFilter fields below describe.
			p := pdr{
				pdrID: 2, fseID: 100, farID: 2,
				srcIface: core, srcIfaceMask: 0xff,
				appFilter: applicationFilter{
					proto: tc.proto, protoMask: tc.protoMask,
					srcPortRange: newWildcardPortRange(),
					dstPortRange: newExactMatchPortRange(port),
				},
			}

			if got := c.matchPDR(p); got != tc.want {
				t.Errorf("coverage = %s, want %s: an exact match means the copy is delivered "+
					"without inspection, so a rule that does not pin the protocol delivers the "+
					"other transport's traffic on that port under this warrant", got, tc.want)
			}
		})
	}
}
