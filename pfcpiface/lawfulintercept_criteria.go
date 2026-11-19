// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/wmnsk/go-pfcp/ie"
)

// This file resolves a Lawful Interception detection criterion (3GPP TS 33.128
// table 6.2.3-7) against the PFCP state this agent already holds: which PDRs, in
// which live sessions, carry traffic the criterion identifies.
//
// It answers only that question. Enabling duplication on the FARs those PDRs
// reference, and dropping copies the enablement over-collected, are separate steps
// — the criterion is resolved once at tasking time, not per packet.

// coverage says how well a PDR's traffic corresponds to a criterion.
type coverage int

const (
	// coverNone: the PDR carries no traffic the criterion identifies.
	coverNone coverage = iota
	// coverExact: every packet the PDR matches is traffic the criterion identifies.
	coverExact
	// coverBroader: the PDR carries the identified traffic and more besides, because
	// its PDI is less specific than the criterion — a criterion naming a transport
	// port against a PDR with no SDF filter, say. Duplicating it collects beyond the
	// criterion, so the copies need filtering before delivery.
	coverBroader
)

func (c coverage) String() string {
	switch c {
	case coverExact:
		return "exact"
	case coverBroader:
		return "broader"
	default:
		return "none"
	}
}

// criterion is a detection criterion parsed into the form the agent compares
// against PFCP state. Parsing happens once, when the task is accepted: the X1
// values arrive as strings, and re-parsing them per PDR — or worse per packet —
// would put string handling on the interception path.
type criterion struct {
	kind types.TargetIdentifierType

	seid uint64 // TargetFSEID
	teid uint32 // TargetFTEID
	// tunnelIP is the F-TEID's address when the criterion carried one. A TEID alone
	// does not identify a tunnel — two nodes may allocate the same value — so the
	// address narrows it where the triggering function supplied it.
	tunnelIP    uint32
	hasTunnelIP bool
	ueIP        uint32 // TargetUEIPv4
	port        uint16 // TargetTCPPort, TargetUDPPort
	proto       uint8  // the transport the port belongs to
	netInstance string // TargetNetworkInstance, as the wire octets
	uplink      bool   // TargetGTPTunnelDirection
	// rule is a whole PDR to compare a session's rules against (TargetPDR). It is
	// held in *this agent's* parsed form rather than as the octets it arrived as:
	// PFCP puts no ordering on the IEs inside a grouped IE, so two encoders can
	// describe one PDR in different bytes, and an octet comparison would miss the
	// match. Parsing both sides with the same parser makes that parser the canonical
	// form — which is also exactly the set of fields this UPF acts on.
	rule *pdr
}

// Transport protocol numbers (IANA), as they appear in an SDF filter's proto field.
const (
	protoTCP uint8 = 6
	protoUDP uint8 = 17
)

// parseCriterion converts an X1 target identifier into a criterion this agent can
// resolve. It returns an error for a criterion the agent cannot evaluate, which
// the caller must report rather than swallow: a criterion silently treated as
// unmatchable would leave an acknowledged interception collecting nothing.
func parseCriterion(t types.TargetIdentifier) (criterion, error) {
	c := criterion{kind: t.Type}

	switch t.Type {
	case types.TargetFSEID:
		seid, err := strconv.ParseUint(t.Value, 10, 64)
		if err != nil {
			return criterion{}, fmt.Errorf("li: invalid PFCP session ID %q", t.Value)
		}
		c.seid = seid

	case types.TargetFTEID:
		// "TEID" or "TEID@address" — the form x1 produces for both the plain
		// gtpuTunnelId identifier and the extension's FTEID.
		val, addr, hasAddr := strings.Cut(t.Value, "@")
		teid, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return criterion{}, fmt.Errorf("li: invalid GTP tunnel ID %q", t.Value)
		}
		c.teid = uint32(teid)
		if hasAddr {
			ip, err := parseIPv4(addr)
			if err != nil {
				return criterion{}, fmt.Errorf("li: invalid GTP tunnel address %q: %w", addr, err)
			}
			c.tunnelIP, c.hasTunnelIP = ip, true
		}

	case types.TargetUEIPv4:
		ip, err := parseIPv4(t.Value)
		if err != nil {
			return criterion{}, fmt.Errorf("li: invalid UE IPv4 address %q: %w", t.Value, err)
		}
		c.ueIP = ip

	case types.TargetTCPPort, types.TargetUDPPort:
		port, err := strconv.ParseUint(t.Value, 10, 16)
		if err != nil || port == 0 {
			return criterion{}, fmt.Errorf("li: invalid port %q", t.Value)
		}
		c.port = uint16(port)
		c.proto = protoTCP
		if t.Type == types.TargetUDPPort {
			c.proto = protoUDP
		}

	case types.TargetPDRID, types.TargetQERID:
		// **Refused, and this is the sharpest refusal in the file.**
		//
		// A PDR ID and a QER ID are allocated *per PFCP session* and reused across sessions
		// from a low number, and matchPDR compares them against every session this element
		// holds. So a task naming PDR 2 would duplicate, attribute and deliver the traffic of
		// every subscriber whose session happens to hold a PDR of that number — under that
		// warrant, indistinguishable downstream from the traffic it did name. That is
		// interception of subjects the warrant does not name, which is the one failure this
		// plane may never have.
		//
		// The library's own type documentation says so: these identifiers "are scoped to
		// a PFCP session, so a criterion using one is only unambiguous alongside the
		// session it belongs to".
		//
		// **Each criterion is judged alone here.** A task's criteria are conjunctive, so an
		// F-SEID beside the rule ID would scope it to one session — but this function sees
		// one criterion at a time, and a rule ID on its own selects that rule in every
		// session. Accepting the pair would need the check made per task, which is not done;
		// until it is, the rule IDs are refused whatever accompanies them. Nor can the filter
		// recover a lone one: every session holding a rule of that number holds it
		// legitimately, so there is nothing in a duplicated copy that distinguishes the
		// intended one.
		//
		// TS 33.128 table 6.2.3-7 lists these among the criteria a CC-POI supports, so
		// refusing them is a declared gap rather than a silent one — README.md and the
		// conformance disposition say which and why. Refusal is also the only conformant
		// option available: accepting a criterion that over-collects is worse than
		// answering that this element cannot honour it.
		return criterion{}, fmt.Errorf(
			"li: %s identifies a rule allocated per PFCP session and reused across sessions, so it "+
				"cannot select one subject's traffic; this element refuses it rather than "+
				"intercepting every session holding a rule of that number", t.Type)

	case types.TargetNetworkInstance:
		// xs:hexBinary on the wire; compared against the octets the PDI carried.
		raw, err := hex.DecodeString(t.Value)
		if err != nil || len(raw) == 0 {
			return criterion{}, fmt.Errorf("li: invalid network instance %q", t.Value)
		}
		c.netInstance = string(raw)

	case types.TargetGTPTunnelDirection:
		// Read relative to this network element, which is the only reading that makes
		// the enumeration usable here: the UPF *receives* uplink GTP packets, so an
		// inbound tunnel is the uplink one, and it *sends* downlink GTP packets, so an
		// outbound tunnel is the downlink one. A PDR's srcIface says which of the two
		// it matches on.
		//
		// The enumeration is two values with no definition attached, so the reading
		// is an interpretation and the cost of having it backwards is intercepting
		// the opposite direction to the one authorised. It is asserted against real
		// traffic end to end rather than left to this comment.
		switch t.Value {
		case x1.GTPDirectionInbound:
			c.uplink = true
		case x1.GTPDirectionOutbound:
			c.uplink = false
		default:
			return criterion{}, fmt.Errorf("li: unknown GTP tunnel direction %q", t.Value)
		}

	case types.TargetUEIPv6:
		// Not an interception limitation: this core has no IPv6 PDU sessions to
		// intercept. The SMF cannot allocate an IPv6 UE address (its allocator is
		// 32-bit, and the NAS accept it would build carries a zero-length address),
		// and a session establishment naming one is rejected here at PDI parse time,
		// where a UE address that is not four octets is an error. So an IPv6
		// criterion could not match anything however it were resolved.
		//
		// Refused rather than accepted-and-never-matched, which would be an
		// interception producing nothing while reporting success.
		return criterion{}, fmt.Errorf("li: this UPF has no IPv6 UE addresses to match")

	case types.TargetPDR:
		rule, err := parsePDRCriterion(t.Value)
		if err != nil {
			return criterion{}, err
		}
		c.rule = rule

	default:
		// Subscriber identities (SUPI, GPSI, PEI) reach the UPF only by mistake: it
		// holds no subscriber identity to match them against.
		return criterion{}, fmt.Errorf("li: %s is not a packet detection criterion", t.Type)
	}

	return c, nil
}

// parsePDRCriterion decodes a PDR detection criterion — an encoded TS 29.244 rule,
// carried as xs:hexBinary — into the form this agent holds its own rules in.
//
// The encoding is taken to be a Create PDR IE, which is the form the SMF sends and
// the only one a triggering function could have obtained the rule from. Anything
// else is refused rather than guessed at.
func parsePDRCriterion(value string) (*pdr, error) {
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("li: PDR criterion is not hexBinary")
	}

	parsed, err := ie.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("li: PDR criterion is not a PFCP information element: %w", err)
	}
	// Create PDR specifically, not merely something that parses as a rule: an Update
	// PDR carries the same fields and would be accepted silently. One agreed form
	// keeps a triggering function from having two ways to say the same thing and
	// getting a different answer.
	if parsed.Type != ie.CreatePDR {
		return nil, fmt.Errorf("li: PDR criterion is IE type %d, want a Create PDR", parsed.Type)
	}

	// A pool of its own, so parsing a criterion cannot take an address from the one
	// real sessions are served from. Nothing is expected to be allocated: a criterion
	// asking the UPF to allocate an address is rejected below.
	pool, err := NewIPPool(criterionScratchPool)
	if err != nil {
		return nil, fmt.Errorf("li: %w", err)
	}

	var rule pdr
	if err := rule.parsePDR(parsed, 0, map[string]appPFD{}, pool); err != nil {
		return nil, fmt.Errorf("li: PDR criterion does not parse as a rule: %w", err)
	}
	if rule.allocIPFlag {
		// The UE IP Address IE asked the UPF to choose an address. That is an
		// instruction, not a description of traffic, and the address it yielded here
		// belongs to a throwaway pool — so this criterion could never match anything.
		return nil, fmt.Errorf("li: PDR criterion leaves the UE address to be allocated")
	}
	if rule.UPAllocateFteid {
		// **The same case through the other field, and the same answer.** A CH F-TEID asks
		// the UPF to choose the tunnel endpoint, so the criterion names a tunnel this
		// element assigns rather than one it can recognise: whatever TEID the session holds,
		// it was not in the criterion, so the rule can never compare equal and the
		// interception is acknowledged and produces nothing.
		//
		// Two answers are legitimate — refuse it, as the UE-address case above
		// is, or normalise the element-assigned fields out of sameRule as the
		// session-assigned ones already are. **Refusal is the decision**, recorded here
		// because it is a choice and not a derivation: a criterion that says "the tunnel the
		// UPF chooses" identifies no traffic on its own, so normalising the field away
		// would leave a criterion matching every session whose remaining fields agree —
		// widening the interception to make an unusable criterion usable. Normalisation
		// stays available if a triggering function ever needs it, and it would need its own
		// requirement, because it changes what a PDR criterion *means*.
		return nil, fmt.Errorf(
			"li: PDR criterion leaves the tunnel endpoint to be allocated by this element, so it " +
				"names no traffic it could match")
	}

	return &rule, nil
}

// criterionScratchPool is the address pool parsePDRCriterion parses against. It is
// never drawn from — a criterion whose rule asks for an allocation is refused — but
// the parser needs a pool to be present in order to reach that refusal rather than
// dereference nil. A fresh one per call, so nothing accumulates, and a loopback range
// so that an address from it appearing anywhere is obviously wrong.
const criterionScratchPool = "127.0.63.0/30"

// sameRule reports whether a session's PDR is the rule a criterion names.
//
// The comparison is over what this agent parses, which has two consequences worth
// being explicit about. Fields the agent does not retain do not participate, so two
// rules differing only in an IE it ignores compare equal. And the fields a *session*
// assigns — the F-SEID it belongs to, the address that allocated it, the counter the
// UPF chose — are excluded, because they are not properties of the rule the
// triggering function described.
func sameRule(want, have pdr) bool {
	want.fseID, have.fseID = 0, 0
	want.fseidIP, have.fseidIP = 0, 0
	want.ctrID, have.ctrID = 0, 0

	return reflect.DeepEqual(want, have)
}

// parseIPv4 converts a dotted-quad address to the datapath's uint32 form.
func parseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return 0, fmt.Errorf("not an IPv4 address")
	}

	return ip2int(ip), nil
}

// pdrRef identifies one PDR of one session, with the FAR it forwards through. The
// FAR is what carries the duplication flag, so it is the thing enablement acts on;
// the PDR is what filtering has to reason about.
type pdrRef struct {
	seid  uint64
	pdrID uint32
	farID uint32
	cover coverage
	// uplink is the direction of the PDR's traffic, which is what lets a copy be
	// attributed without inspecting it: the datapath's tag says which direction each
	// copy came from.
	uplink bool
}

func (r pdrRef) String() string {
	return fmt.Sprintf("pdr(seid=%d, id=%d, far=%d, %s)", r.seid, r.pdrID, r.farID, r.cover)
}

// resolve returns every PDR, across all live sessions, that carries traffic the
// criterion identifies, each with how closely it corresponds. It walks all
// sessions rather than one because most criteria do not name a session: a network
// instance spans many, and an address or tunnel identifies one without saying
// which.
//
// An empty result means the criterion selects nothing *at present*. That is not
// the same as unevaluable — a criterion may name a subscriber that has not yet
// attached — so the caller must distinguish "no session matches yet" from the
// error parseCriterion returns.
func (c criterion) resolve(sessions []PFCPSession) []pdrRef {
	var out []pdrRef

	for i := range sessions {
		for j := range sessions[i].pdrs {
			p := sessions[i].pdrs[j]
			if cov := c.matchPDR(p); cov != coverNone {
				out = append(out, pdrRef{
					seid: p.fseID, pdrID: p.pdrID, farID: p.farID,
					cover: cov, uplink: p.IsUplink(),
				})
			}
		}
	}

	return out
}

// selection returns every PDR, across the given sessions, that carries traffic all of a
// task's criteria identify, each with how closely it corresponds.
//
// A task's criteria are conjunctive — ETSI TS 103 221-1 table 6.2.1.2-1: "Where multiple
// criteria are present, all criteria are required to be matched" — so a PDR is selected
// only where every criterion selects it, and its coverage is exact only where every
// criterion's is: a criterion that covers the PDR more broadly than itself, a transport
// port on a PDR whose filter does not pin it, leaves copies of that PDR to be decided
// packet by packet. A task with no criteria selects nothing.
func selection(criteria []criterion, sessions []PFCPSession) []pdrRef {
	if len(criteria) == 0 {
		return nil
	}

	var out []pdrRef

	for i := range sessions {
		for j := range sessions[i].pdrs {
			p := sessions[i].pdrs[j]
			cov := coverExact
			for _, c := range criteria {
				switch c.matchPDR(p) {
				case coverNone:
					cov = coverNone
				case coverBroader:
					cov = coverBroader
				default:
				}
				if cov == coverNone {
					break
				}
			}
			if cov == coverNone {
				continue
			}
			out = append(out, pdrRef{
				seid: p.fseID, pdrID: p.pdrID, farID: p.farID,
				cover: cov, uplink: p.IsUplink(),
			})
		}
	}

	return out
}

// matchPDR reports how a PDR's traffic corresponds to the criterion.
func (c criterion) matchPDR(p pdr) coverage {
	switch c.kind {
	case types.TargetFSEID:
		// Every PDR of the session carries its traffic, and the criterion is the
		// session, so nothing broader is included.
		return exactIf(p.fseID == c.seid)

	case types.TargetFTEID:
		// A tunnel endpoint appears in the PDI of the PDR that matches on it, which is
		// the uplink one: the downlink PDR of the same session matches on the UE
		// address instead and its traffic leaves through a tunnel the PDI does not
		// name. So this selects one direction, exactly.
		if p.tunnelTEID != c.teid || p.tunnelTEIDMask == 0 {
			return coverNone
		}
		if c.hasTunnelIP && p.tunnelIP4Dst != c.tunnelIP {
			return coverNone
		}

		return coverExact

	case types.TargetUEIPv4:
		return exactIf(p.ueAddress == c.ueIP && p.ueAddress != 0)

	case types.TargetTCPPort, types.TargetUDPPort:
		return c.matchPort(p)

	case types.TargetPDRID, types.TargetQERID:
		// Unreachable: parseCriterion refuses these, because a rule identifier is
		// allocated per PFCP session and reused, so matching it here selected the rule of
		// that number in *every* session this element holds. Left as an explicit
		// coverNone rather than deleted, so a criterion that somehow reached here matches
		// nothing instead of falling through to the default and matching by accident.
		return coverNone

	case types.TargetNetworkInstance:
		return exactIf(p.networkInstance != "" && p.networkInstance == c.netInstance)

	case types.TargetGTPTunnelDirection:
		return exactIf(p.IsUplink() == c.uplink)

	case types.TargetPDR:
		// The criterion is a rule, so a PDR either is that rule or is not: there is
		// nothing broader about it.
		return exactIf(c.rule != nil && sameRule(*c.rule, p))

	default:
		// parseCriterion refuses everything else, so reaching here means a criterion
		// was constructed without going through it.
		return coverNone
	}
}

// matchPort resolves a transport-port criterion against a PDR's SDF filter. The
// UE is the source of uplink traffic and the destination of downlink traffic, so
// which port range describes the UE's port depends on the PDR's direction.
//
// A PDR whose filter does not constrain the port still carries the port's traffic,
// so it is selected as broader rather than rejected: refusing it would mean an
// interception ordered by port produces nothing on a session whose rules happen to
// be wildcard, which is the common case.
func (c criterion) matchPort(p pdr) coverage {
	// **Coverage is computed from every dimension the criterion constrains, as a
	// conjunction, rather than returned early from the first one checked.**
	//
	// A transport-port criterion constrains two things: the protocol and the port. If the
	// protocol were used only to *exclude* — a rule pinning a different protocol matches
	// nothing — and never to widen, a rule that pinned the port exactly and left the
	// protocol unconstrained would return coverExact. Exact means the copy is delivered
	// without inspection, so a TCP-port criterion would deliver the UDP traffic on that port
	// too: traffic the warrant does not name, under the warrant's own identifier.
	//
	// Written as a conjunction so that a dimension added later cannot silently reintroduce
	// an exact match that is not exact: each dimension reports whether the rule pins it, and
	// coverage is exact only where every one of them does.
	if p.appFilter.protoMask != 0 && p.appFilter.proto != c.proto {
		return coverNone
	}

	r := p.appFilter.dstPortRange
	if p.IsUplink() {
		r = p.appFilter.srcPortRange
	}

	// The port dimension: does the rule carry this port, and does it carry only this port.
	switch {
	case r.isWildcardMatch():
		// No port constraint: the traffic is in there, along with every other port.
		return coverBroader
	case c.port < r.low || c.port > r.high:
		return coverNone
	}
	portExact := r.isExactMatch()

	// The protocol dimension: the criterion names one, so a rule that does not pin it
	// carries the other transports' traffic on the same port as well.
	protoExact := p.appFilter.protoMask != 0

	if portExact && protoExact {
		return coverExact
	}

	// Anything else is broader than the criterion in at least one dimension, so the copy is
	// inspected rather than assumed. The cost is a packet inspection on rules that pin a
	// port and not a protocol, on a path that is already per copy of already-duplicated
	// traffic — and the alternative is delivering traffic the warrant does not name.
	return coverBroader
}

func exactIf(ok bool) coverage {
	if ok {
		return coverExact
	}

	return coverNone
}

// farRef identifies one FAR of one session.
type farRef struct {
	seid  uint64
	farID uint32
}
