// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package fake_bess

import (
	"context"
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/protobuf/types/known/anypb"
)

func intField(v uint64) *bess_pb.FieldData {
	return &bess_pb.FieldData{Encoding: &bess_pb.FieldData_ValueInt{ValueInt: v}}
}

// The value a FAR carries into the datapath is not the PFCP Apply Action. The agent
// translates the action into the gate the farLookup module switches on, and that is what
// lands in the table and what comes back out of it.

// farAddArg is the shape addFAR writes: two key fields, then the action followed by the
// tunnel values.
func farAddArg(action uint64) *bess_pb.ExactMatchCommandAddArg {
	return &bess_pb.ExactMatchCommandAddArg{
		Fields: []*bess_pb.FieldData{intField(1), intField(0xC0FFEE)},
		Values: []*bess_pb.FieldData{
			intField(action),
			intField(0), intField(0), intField(0), intField(0), intField(0),
		},
	}
}

func TestAFarReportsTheActionTheDatapathWasGiven(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		action                   uint64
		drops, forwards, buffers bool
	}{
		{name: "forwards downlink", action: 0, forwards: true},
		{name: "forwards uplink", action: 1, forwards: true},
		{name: "drops", action: 2, drops: true},
		{name: "notifies", action: 4, buffers: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			far := UnmarshalFar(farAddArg(tc.action))

			if got := far.Drops(); got != tc.drops {
				t.Errorf("Drops() = %v for action %d, expected %v", got, tc.action, tc.drops)
			}

			if got := far.Forwards(); got != tc.forwards {
				t.Errorf("Forwards() = %v for action %d, expected %v", got, tc.action, tc.forwards)
			}

			if got := far.Buffers(); got != tc.buffers {
				t.Errorf("Buffers() = %v for action %d, expected %v", got, tc.action, tc.buffers)
			}
		})
	}
}

// A test reads the tables on its own goroutine while the agent programs them from a gRPC
// handler on another, and the PFCP round trip between the two is a socket -- which is not
// an ordering the race detector can see. So the read has to take its copy under the same
// lock the handler writes under, and the copy has to be a real one: an add that matches
// an entry already there resets and rewrites that message in place.

// pdrAddArg is the shape addPDR writes: eight match values with their masks, and the
// five rule values behind them, of which the first is the PDR ID.
func pdrAddArg(pdrID uint64) *anypb.Any {
	values := make([]*bess_pb.FieldData, 0, 8)
	masks := make([]*bess_pb.FieldData, 0, 8)

	for i := 0; i < 8; i++ {
		values = append(values, intField(pdrID))
		masks = append(masks, intField(0xFFFFFFFF))
	}

	arg, err := anypb.New(&bess_pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values:   values,
		Masks:    masks,
		Valuesv: []*bess_pb.FieldData{
			intField(pdrID), intField(0xC0FFEE), intField(0), intField(0), intField(0),
		},
	})
	if err != nil {
		panic(err)
	}

	return arg
}

func TestTheTablesCanBeReadWhileTheyAreBeingProgrammed(t *testing.T) {
	const rounds = 200

	b := NewFakeBESS()

	programmed := make(chan struct{})

	go func() {
		defer close(programmed)

		for i := 0; i < rounds; i++ {
			// Half the adds repeat an earlier key, which is the path that rewrites a
			// stored message rather than appending a new one.
			_, err := b.service.ModuleCommand(context.Background(), &bess_pb.CommandRequest{
				Name: pdrLookupModuleName,
				Cmd:  addCmd,
				Arg:  pdrAddArg(uint64(i % (rounds / 2))),
			})
			if err != nil {
				t.Errorf("programming the PDR table failed: %v", err)
				return
			}
		}
	}()

	// Wait for the writer even when an assertion returns early, so a failure is
	// reported as itself rather than as a Fail after the test has completed.
	defer func() { <-programmed }()

	// Keep reading until the writer is done. A fixed number of reads finishes long
	// before the adds do, so it never overlaps the second half of them -- which is the
	// half that rewrites entries in place rather than appending new ones.
	for done := false; !done; {
		select {
		case <-programmed:
			done = true
		default:
		}

		for id, entries := range b.GetPdrTableEntries() {
			// Read a field of every entry, which is what a caller does with them. The
			// assertion is what makes this a real read; the detector is what the test
			// is for.
			for _, e := range entries {
				if e.PdrID != id {
					t.Errorf("entry under key %d reports PDR ID %d", id, e.PdrID)
					return
				}
			}
		}
	}
}
