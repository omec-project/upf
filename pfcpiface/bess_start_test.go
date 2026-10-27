// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"net"
	"slices"
	"sync"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

// commandRecordingBESS is a BESS gRPC server that accepts every module command and
// records which module was sent which command.
type commandRecordingBESS struct {
	pb.UnimplementedBESSControlServer

	mu       sync.Mutex
	commands []string
}

func (s *commandRecordingBESS) ModuleCommand(
	_ context.Context, req *pb.CommandRequest,
) (*pb.CommandResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commands = append(s.commands, req.GetName()+" "+req.GetCmd())

	return &pb.CommandResponse{}, nil
}

func (s *commandRecordingBESS) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.commands)
}

// startRecordingBESS serves a commandRecordingBESS on a local port and points the
// agent at it for the rest of the test.
func startRecordingBESS(t *testing.T) *commandRecordingBESS {
	t.Helper()

	var lc net.ListenConfig

	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	rec := &commandRecordingBESS{}
	srv := grpc.NewServer()
	pb.RegisterBESSControlServer(srv, rec)

	served := make(chan error, 1)

	go func() { served <- srv.Serve(lis) }()

	t.Cleanup(func() {
		srv.Stop()

		if err := <-served; err != nil {
			t.Errorf("recording BESS: %v", err)
		}
	})

	savedBessIP := *bessIP
	savedPathMonitoring := enableGtpuPathMonitoring

	t.Cleanup(func() {
		*bessIP = savedBessIP
		enableGtpuPathMonitoring = savedPathMonitoring
	})

	*bessIP = lis.Addr().String()

	return rec
}

// The tests here write the globals *bessIP and enableGtpuPathMonitoring, so they must
// not run in parallel.

// A pfcp-agent clears the datapath when it starts. bessd runs in its own container, so it
// can outlive a restart of the agent; its gtpuPathMonitoring module has to be cleared
// with the rest, or it goes on monitoring the gNBs of sessions the agent no longer has.
func TestSetUpfInfoClearsPathMonitoringOnStart(t *testing.T) {
	rec := startRecordingBESS(t)
	enableGtpuPathMonitoring = false // a freshly started agent

	b := &bess{}
	b.SetUpfInfo(&upf{}, &Conf{EnableGtpuPathMonitoring: true})
	t.Cleanup(b.Exit)

	if sent := rec.sent(); !slices.Contains(sent, "gtpuPathMonitoring clear") {
		t.Errorf("SetUpfInfo did not clear gtpuPathMonitoring; module commands sent: %v", sent)
	}

	if !enableGtpuPathMonitoring {
		t.Error("SetUpfInfo did not enable path monitoring")
	}
}

// Without path monitoring configured, the module may not exist, so nothing is sent to it.
func TestSetUpfInfoLeavesPathMonitoringAloneWhenDisabled(t *testing.T) {
	rec := startRecordingBESS(t)
	enableGtpuPathMonitoring = false

	b := &bess{}
	b.SetUpfInfo(&upf{}, &Conf{})
	t.Cleanup(b.Exit)

	sent := rec.sent()
	if !slices.Contains(sent, "pdrLookup clear") {
		t.Fatalf("SetUpfInfo did not reach the datapath; module commands sent: %v", sent)
	}

	if slices.Contains(sent, "gtpuPathMonitoring clear") {
		t.Errorf("SetUpfInfo cleared gtpuPathMonitoring with path monitoring disabled; sent: %v", sent)
	}
}
