// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"maps"
	"net"
	"reflect"
	"sync"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
)

// pathMonitoringRecorder answers every module command and records those sent to
// gtpuPathMonitoring, keeping the module's own count per address: add increments,
// delete decrements, and the address stops being monitored at zero. A delete of an
// address that is not monitored is recorded as a fault.
type pathMonitoringRecorder struct {
	pb.BESSControlClient

	refuseFARAdd bool

	mu       sync.Mutex
	commands []string
	count    map[string]int
	faults   []string
}

func (r *pathMonitoringRecorder) ModuleCommand(
	_ context.Context, req *pb.CommandRequest, _ ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	if r.refuseFARAdd && req.GetName() == "farLookup" && req.GetCmd() == "add" {
		return &pb.CommandResponse{Error: &pb.Error{Code: 1, Errmsg: "refused"}}, nil
	}

	if req.GetName() != "gtpuPathMonitoring" || req.GetCmd() == "clear" {
		return &pb.CommandResponse{}, nil
	}

	a := &pb.GtpuPathMonitoringCommandAddDeleteArg{}
	if err := req.GetArg().UnmarshalTo(a); err != nil {
		return nil, err
	}

	ip := int2ip(a.GetGnbIp()).String()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.commands = append(r.commands, req.GetCmd()+" "+ip)

	switch req.GetCmd() {
	case "add":
		r.count[ip]++
	case "delete":
		if r.count[ip] == 0 {
			r.faults = append(r.faults, "delete of unmonitored "+ip)
			break
		}

		if r.count[ip]--; r.count[ip] == 0 {
			delete(r.count, ip)
		}
	}

	return &pb.CommandResponse{}, nil
}

func (r *pathMonitoringRecorder) monitored() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return maps.Clone(r.count)
}

func (r *pathMonitoringRecorder) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.commands...)
}

func newPathMonitoringBess(t *testing.T) (*bess, *pathMonitoringRecorder) {
	t.Helper()

	saved := enableGtpuPathMonitoring
	enableGtpuPathMonitoring = true

	t.Cleanup(func() { enableGtpuPathMonitoring = saved })

	r := &pathMonitoringRecorder{count: map[string]int{}}

	return &bess{client: r}, r
}

const (
	gnbA = "10.200.0.201"
	gnbB = "10.200.0.202"

	// GTP-U/UDP/IPv4 in the Outer Header Creation Description, as the SMF sends it.
	ohcGtpUUDPIPv4 = 0x0100
)

// pfcpFAR parses a FAR the way the session handlers do, so the test's FARs are ones
// pfcpiface can produce rather than guesses at its fields.
func pfcpFAR(t *testing.T, farIE *ie.IE, seid uint64, op operation) far {
	t.Helper()

	var f far

	u := &upf{accessIP: net.ParseIP("192.168.0.1"), coreIP: net.ParseIP("10.0.10.1")}
	if err := f.parseFAR(farIE, seid, u, op); err != nil {
		t.Fatalf("parseFAR: %v", err)
	}

	return f
}

// The FARs the SD-Core SMF gives a session on one UPF (smf context/datapath.go,
// producer/n1n2_data_handler.go, pfcp/message/build.go): an uplink FAR to the core,
// and a downlink FAR that drops until the gNB's tunnel is known, then forwards to it,
// buffers with no tunnel while the UE is idle, and forwards again after a service
// request.
type sessionFARs struct {
	ul, dlCreated, dlIdle far
	dlTo                  func(gnb string) far
}

func newSessionFARs(t *testing.T, seid uint64) sessionFARs {
	t.Helper()

	const ulID, dlID = 1, 2

	accessFwd := func(extra ...*ie.IE) *ie.IE {
		return ie.NewUpdateForwardingParameters(append([]*ie.IE{
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewNetworkInstance("internet"),
		}, extra...)...)
	}

	return sessionFARs{
		ul: pfcpFAR(t, ie.NewCreateFAR(
			ie.NewFARID(ulID),
			ie.NewApplyAction(ActionForward),
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
				ie.NewNetworkInstance("internet"),
			),
		), seid, create),
		dlCreated: pfcpFAR(t, ie.NewCreateFAR(
			ie.NewFARID(dlID),
			ie.NewApplyAction(ActionDrop),
		), seid, create),
		dlIdle: pfcpFAR(t, ie.NewUpdateFAR(
			ie.NewFARID(dlID),
			ie.NewApplyAction(ActionBuffer|ActionNotify),
			accessFwd(),
		), seid, update),
		dlTo: func(gnb string) far {
			return pfcpFAR(t, ie.NewUpdateFAR(
				ie.NewFARID(dlID),
				ie.NewApplyAction(ActionForward),
				accessFwd(ie.NewOuterHeaderCreation(ohcGtpUUDPIPv4, 0x10, gnb, "", 0, 0, 0)),
			), seid, update)
		},
	}
}

func establish(b *bess, fars ...far) {
	b.SendMsgToUPF(upfMsgTypeAdd, PacketForwardingRules{fars: fars}, PacketForwardingRules{})
}

func modify(b *bess, session []far, updated ...far) {
	b.SendMsgToUPF(upfMsgTypeMod, PacketForwardingRules{fars: session}, PacketForwardingRules{fars: updated})
}

func release(b *bess, session ...far) {
	b.SendMsgToUPF(upfMsgTypeDel, PacketForwardingRules{fars: session}, PacketForwardingRules{})
}

func expectMonitored(t *testing.T, r *pathMonitoringRecorder, when string, want map[string]int) {
	t.Helper()

	if got := r.monitored(); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: monitored %v, want %v (commands %v)", when, got, want, r.sent())
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, f := range r.faults {
		t.Errorf("%s: %s (commands %v)", when, f, r.commands)
	}

	r.faults = nil
}

// A gNB is monitored while a session's FAR forwards to it, and a FAR without a tunnel,
// such as the uplink or a buffering downlink, is not monitored at all.
func TestPathMonitoringFollowsASessionsLife(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	s := newSessionFARs(t, 7)
	dl := s.dlTo(gnbA)

	establish(b, s.ul, s.dlCreated)
	expectMonitored(t, r, "after establishment", map[string]int{})

	modify(b, []far{s.ul, dl}, dl)
	expectMonitored(t, r, "after the gNB's tunnel is set", map[string]int{gnbA: 1})

	modify(b, []far{s.ul, s.dlIdle}, s.dlIdle)
	expectMonitored(t, r, "while the UE is idle", map[string]int{})

	modify(b, []far{s.ul, dl}, dl)
	expectMonitored(t, r, "after a service request", map[string]int{gnbA: 1})

	release(b, s.ul, dl)
	expectMonitored(t, r, "after release", map[string]int{})

	want := []string{"add " + gnbA, "delete " + gnbA, "add " + gnbA, "delete " + gnbA}
	if got := r.sent(); !reflect.DeepEqual(got, want) {
		t.Errorf("commands %v, want %v", got, want)
	}
}

func TestPathMonitoringMovesWithAHandover(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	s := newSessionFARs(t, 7)
	dlA, dlB := s.dlTo(gnbA), s.dlTo(gnbB)

	establish(b, s.ul, s.dlCreated)
	modify(b, []far{s.ul, dlA}, dlA)
	modify(b, []far{s.ul, dlB}, dlB)
	expectMonitored(t, r, "after the handover", map[string]int{gnbB: 1})

	release(b, s.ul, dlB)
	expectMonitored(t, r, "after release", map[string]int{})
}

// The module's count is per address, so a gNB stays monitored while any session uses it.
func TestPathMonitoringKeepsAGnbWhileAnySessionUsesIt(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	s1, s2 := newSessionFARs(t, 7), newSessionFARs(t, 8)
	dl1, dl2 := s1.dlTo(gnbA), s2.dlTo(gnbA)

	establish(b, s1.ul, dl1)
	establish(b, s2.ul, dl2)
	expectMonitored(t, r, "with two sessions", map[string]int{gnbA: 2})

	release(b, s1.ul, dl1)
	expectMonitored(t, r, "after the first release", map[string]int{gnbA: 1})

	release(b, s2.ul, dl2)
	expectMonitored(t, r, "after the second release", map[string]int{})
}

// A refused modification is undone by writing the session's previous rules again
// (handleSessionModificationRequest). That must not register an unchanged FAR's gNB a
// second time, and a FAR the refused modification had moved must move back.
func TestPathMonitoringFollowsARestore(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	s := newSessionFARs(t, 7)
	dlA, dlB := s.dlTo(gnbA), s.dlTo(gnbB)
	before := []far{s.ul, dlA}

	establish(b, before...)
	modify(b, before, before...)
	expectMonitored(t, r, "after restoring unchanged rules", map[string]int{gnbA: 1})

	modify(b, []far{s.ul, dlB}, dlB)
	modify(b, before, before...)
	expectMonitored(t, r, "after restoring a moved FAR", map[string]int{gnbA: 1})
}

// A FAR the datapath refused was not programmed, so nothing monitors its gNB.
func TestPathMonitoringSkipsARefusedFAR(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	r.refuseFARAdd = true
	s := newSessionFARs(t, 7)

	establish(b, s.ul, s.dlTo(gnbA))
	expectMonitored(t, r, "after a refused establishment", map[string]int{})
}

// Clearing the datapath clears the module too, so the record of what each FAR registered
// has to go with it: otherwise the next session reusing that F-SEID would not register.
func TestPathMonitoringStartsAgainAfterAClear(t *testing.T) {
	b, r := newPathMonitoringBess(t)
	s := newSessionFARs(t, 7)
	dl := s.dlTo(gnbA)

	establish(b, s.ul, dl)
	b.clearState()

	r.mu.Lock()
	r.count = map[string]int{} // what the module's clear does
	r.mu.Unlock()

	establish(b, s.ul, dl)
	expectMonitored(t, r, "after a clear and a new session", map[string]int{gnbA: 1})
}
