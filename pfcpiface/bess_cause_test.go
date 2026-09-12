// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The cause SendMsgToUPF returns is what every caller in messages_session.go compares
// against ie.CauseRequestRejected to decide whether the session exists. A batch that
// did not complete must therefore not be reported as accepted.

// newCauseTestBess builds the datapath the way SetUpfInfo does, so a QER finds the
// default QCI entry readQciQosMap guarantees.
func newCauseTestBess() *bess {
	b := &bess{client: moduleCommandStub{resp: &pb.CommandResponse{}}}
	b.readQciQosMap(&Conf{})

	return b
}

// unprogrammableRule is refused by CreatePortRangeCartesianProduct, so addPDR reports
// its failure without reaching the datapath client at all.
func unprogrammableRule() pdr {
	p := pdr{}
	p.appFilter.srcPortRange = newRangeMatchPortRange(100, 200)
	p.appFilter.dstPortRange = newRangeMatchPortRange(300, 400)

	return p
}

func TestSendMsgToUPFRejectsABatchThatDidNotComplete(t *testing.T) {
	b := newCauseTestBess()

	rules := PacketForwardingRules{pdrs: []pdr{unprogrammableRule()}}

	if cause := b.SendMsgToUPF(upfMsgTypeAdd, rules, PacketForwardingRules{}); cause != ie.CauseRequestRejected {
		t.Errorf("SendMsgToUPF() = %d for a rule that was never programmed, want CauseRequestRejected (%d)",
			cause, ie.CauseRequestRejected)
	}
}

// A session carries rules of all three kinds, and the ones that programmed fine must not
// mask the one that did not: a session holding any unprogrammed rule cannot forward what
// the control plane thinks it can.
func TestSendMsgToUPFRejectsASessionWithOneUnprogrammedRule(t *testing.T) {
	b := newCauseTestBess()

	rules := PacketForwardingRules{
		pdrs: []pdr{{}, unprogrammableRule(), {}},
		fars: []far{{}},
		qers: []qer{{}},
	}

	if cause := b.SendMsgToUPF(upfMsgTypeAdd, rules, PacketForwardingRules{}); cause != ie.CauseRequestRejected {
		t.Errorf("SendMsgToUPF() = %d for a session with one unprogrammed rule, want CauseRequestRejected (%d)",
			cause, ie.CauseRequestRejected)
	}
}

func TestSendMsgToUPFAcceptsABatchTheDatapathTook(t *testing.T) {
	b := newCauseTestBess()

	rules := PacketForwardingRules{
		pdrs: []pdr{{}},
		fars: []far{{}},
		qers: []qer{{}},
	}

	if cause := b.SendMsgToUPF(upfMsgTypeAdd, rules, PacketForwardingRules{}); cause != ie.CauseRequestAccepted {
		t.Errorf("SendMsgToUPF() = %d for a batch every rule of which was programmed, want CauseRequestAccepted (%d)",
			cause, ie.CauseRequestAccepted)
	}
}

// A message that carries no rules never reaches the join, and answering it as accepted is
// the behaviour every caller already relies on.
func TestSendMsgToUPFAcceptsAnEmptyBatch(t *testing.T) {
	b := &bess{}

	if cause := b.SendMsgToUPF(upfMsgTypeAdd, PacketForwardingRules{}, PacketForwardingRules{}); cause != ie.CauseRequestAccepted {
		t.Errorf("SendMsgToUPF() = %d for a message carrying no rules, want CauseRequestAccepted (%d)",
			cause, ie.CauseRequestAccepted)
	}
}

// blockingModuleStub holds every datapath call until it is released, so a batch cannot
// complete within Timeout. The rule goroutines report once released, which the buffered
// done channel absorbs after the join has already given up.
type blockingModuleStub struct {
	pb.BESSControlClient

	release chan struct{}
}

func (s blockingModuleStub) ModuleCommand(
	_ context.Context, _ *pb.CommandRequest, _ ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	<-s.release

	return &pb.CommandResponse{}, nil
}

// A batch that did not complete must not be answered as rejected. Nothing here knows what
// a timed-out batch left in the datapath, and on the establishment path a rejection has
// the caller remove the session -- so it would forget one whose rules may still be
// installed and forwarding. Accepted is not a claim that the rules landed; it is the
// answer that does not destroy state we cannot see.
func TestSendMsgToUPFDoesNotRejectABatchThatDidNotComplete(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	b := &bess{client: blockingModuleStub{release: release}}
	b.readQciQosMap(&Conf{})

	rules := PacketForwardingRules{pdrs: []pdr{{}}}

	if cause := b.SendMsgToUPF(upfMsgTypeAdd, rules, PacketForwardingRules{}); cause != ie.CauseRequestAccepted {
		t.Errorf("SendMsgToUPF() = %d for a batch that timed out, want CauseRequestAccepted (%d); "+
			"a timed-out batch has an unknown subset of its rules programmed, so rejecting it "+
			"makes the caller discard a session the datapath may still hold",
			cause, ie.CauseRequestAccepted)
	}
}

// answeringBess builds the datapath the way newCauseTestBess does, but with a client
// that answers every module command the same way.
func answeringBess(resp *pb.CommandResponse, err error) *bess {
	b := &bess{client: moduleCommandStub{resp: resp, err: err}}
	b.readQciQosMap(&Conf{})

	return b
}

// errnoNoSpace is ENOSPC: what the modules answer for a table that is full, which is the
// refusal this whole chain exists to carry.
const errnoNoSpace = 28

// A module that refuses a rule answers over a *healthy* RPC: the gRPC error is nil and the
// refusal rides in the response. Until the workers reported what process* observed, that
// answer was logged and dropped -- the batch completed, every worker claimed it had
// programmed its rule, and the session was answered accepted while the datapath held none
// of it. A full table is the case that matters: it is reported by the modules and it
// persists, so every session after it is answered the same wrong way.
func TestSendMsgToUPFRejectsARuleTheModuleRefused(t *testing.T) {
	refused := &pb.CommandResponse{Error: &pb.Error{Code: errnoNoSpace, Errmsg: "table is full"}}

	for _, tc := range []struct {
		name  string
		rules PacketForwardingRules
	}{
		{"PDR", PacketForwardingRules{pdrs: []pdr{{}}}},
		{"FAR", PacketForwardingRules{fars: []far{{}}}},
		{"QER", PacketForwardingRules{qers: []qer{{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := answeringBess(refused, nil)

			if cause := b.SendMsgToUPF(upfMsgTypeAdd, tc.rules, PacketForwardingRules{}); cause != ie.CauseRequestRejected {
				t.Errorf("SendMsgToUPF() = %d for a %s the module refused, want CauseRequestRejected (%d)",
					cause, tc.name, ie.CauseRequestRejected)
			}
		})
	}
}

// A delete is the exception, and it has to be: "the rule is not there" is the state a
// delete is asking for. A batch that timed out is answered accepted with an unknown
// subset of its rules in place, and the session's later deletion names all of them, so a
// delete legitimately reaches for rules that were never programmed; treating that as a
// refusal would reject deletions that did exactly what they were asked. ExactMatch
// reports ENOENT for a rule it does not hold and WildcardMatch for a mask it has no rule
// under, while the fake datapath reports the gRPC codes.NotFound, so both shapes have to
// be recognised.
func TestSendMsgToUPFAcceptsADeleteOfARuleTheDatapathDoesNotHold(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp *pb.CommandResponse
		err  error
	}{
		{"module answers ENOENT", &pb.CommandResponse{Error: &pb.Error{Code: errnoNoEntry, Errmsg: "rule doesn't exist"}}, nil},
		{"RPC answers NotFound", nil, status.Error(codes.NotFound, "entry not found")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := answeringBess(tc.resp, tc.err)

			rules := PacketForwardingRules{pdrs: []pdr{{}}, fars: []far{{}}, qers: []qer{{}}}

			if cause := b.SendMsgToUPF(upfMsgTypeDel, rules, PacketForwardingRules{}); cause != ie.CauseRequestAccepted {
				t.Errorf("SendMsgToUPF() = %d deleting rules the datapath does not hold, want CauseRequestAccepted (%d); "+
					"a delete that finds nothing has reached the state it asked for",
					cause, ie.CauseRequestAccepted)
			}
		})
	}
}

// The exception is only for the rule being absent. A module that refuses to remove a rule
// it does hold leaves the datapath forwarding traffic the control plane believes it has
// stopped, and that has to reach the cause.
func TestSendMsgToUPFRejectsADeleteTheModuleRefused(t *testing.T) {
	b := answeringBess(&pb.CommandResponse{Error: &pb.Error{Code: errnoNoSpace, Errmsg: "cannot delete"}}, nil)

	rules := PacketForwardingRules{pdrs: []pdr{{}}}

	if cause := b.SendMsgToUPF(upfMsgTypeDel, rules, PacketForwardingRules{}); cause != ie.CauseRequestRejected {
		t.Errorf("SendMsgToUPF() = %d for a delete the module refused, want CauseRequestRejected (%d)",
			cause, ie.CauseRequestRejected)
	}
}
