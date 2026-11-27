// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/upf-epc/logger"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The datapath write duration is a general operator metric on a path that also carries
// interception rules. Undetectability forbids the general plane from revealing that
// interception is happening, and a metric that could be tied to one subscriber would say
// more than a general metric may — so this asserts what it exposes rather than trusting
// that the declaration stays neutral.
func TestDatapathWriteDurationDisclosesNoSubject(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(datapathWriteDuration); err != nil {
		t.Fatalf("register: %v", err)
	}

	// The vec is package-level and every bess.SendMsgToUPF in the package's other tests
	// observes into it, so without this the family holds one child per method those tests
	// happened to use and the assertions below read whatever ran first.
	datapathWriteDuration.Reset()
	t.Cleanup(datapathWriteDuration.Reset)

	datapathWriteDuration.WithLabelValues(upfMsgTypeMod.String()).Observe(0.01)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("expected exactly one metric family, got %d", len(families))
	}

	mf := families[0]
	// Anything that could name a subject, a session, or the rules of one. "li" and
	// "intercept" are here because the name itself must not disclose the plane either.
	forbidden := []string{
		"seid", "fseid", "session", "far", "pdr", "qer", "teid",
		"supi", "imsi", "suci", "msisdn", "ue", "subscriber", "target",
		"li", "intercept", "duplicat", "warrant", "xid",
	}

	check := func(where, s string) {
		low := strings.ToLower(s)
		for _, f := range forbidden {
			if strings.Contains(low, f) {
				t.Errorf("%s %q contains %q — this metric must not disclose a subject or the interception plane", where, s, f)
			}
		}
	}

	check("metric name", mf.GetName())

	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			check("label name", lp.GetName())
			check("label value", lp.GetValue())
		}
	}

	// Every child's label set must be exactly the PFCP procedure, so a later author
	// cannot widen it into something correlatable without this failing. Asserted per
	// child rather than over the flattened list: the number of children is how many
	// methods have been observed, which is not what this test is about.
	for _, m := range mf.GetMetric() {
		var got []string
		for _, lp := range m.GetLabel() {
			got = append(got, lp.GetName())
		}
		if len(got) != 1 || got[0] != "method" {
			t.Errorf("label set = %v, want exactly [method]", got)
		}
		if m.GetHistogram() == nil {
			t.Errorf("expected a histogram, got %v", m)
		}
	}
}

// A write the interception plane makes on its own, outside any PFCP procedure, is not
// timed. A modify observation with no PFCP Session Modification behind it would let anyone
// who compares this histogram with the PFCP message counters see that a tasking changed.
func TestAnInterceptionPushIsNotTimed(t *testing.T) {
	datapathWriteDuration.Reset()
	t.Cleanup(datapathWriteDuration.Reset)

	u := &upf{datapath: bessWithNoDatapath(t)}
	rules := PacketForwardingRules{fars: []far{{farID: 1, fseID: 0x2632898145f4d191, applyAction: ActionForward}}}

	u.sendDuplicationPush(PacketForwardingRules{}, rules)

	if n := testutil.CollectAndCount(datapathWriteDuration); n != 0 {
		t.Errorf("the interception push was timed: %d series after it, want 0", n)
	}

	// The same write made by a PFCP Session Modification is timed, so the count above can
	// only be zero because the push was left out.
	u.sendDuplicationWrite(upfMsgTypeMod, PacketForwardingRules{}, rules)

	if n := testutil.CollectAndCount(datapathWriteDuration); n != 1 {
		t.Errorf("a modification's write left %d series, want 1", n)
	}
}

// The enabler the shipper starts pushes through that untimed path. Without this, the wiring
// could go back to the timed write with every other test still passing.
func TestTheInterceptionEnablerPushesUntimed(t *testing.T) {
	datapathWriteDuration.Reset()
	t.Cleanup(datapathWriteDuration.Reset)

	u := &upf{datapath: bessWithNoDatapath(t)}
	e := u.newInterceptionEnabler(func(string, string) {})
	t.Cleanup(e.stop)

	rules := PacketForwardingRules{fars: []far{{farID: 1, fseID: 0x2632898145f4d191, applyAction: ActionForward}}}
	e.push(PacketForwardingRules{}, rules)

	if n := testutil.CollectAndCount(datapathWriteDuration); n != 0 {
		t.Errorf("the enabler's push was timed: %d series after it, want 0", n)
	}
}

// slowRuleLog is a debug sink that is slow only for the line the batch logs for each rule as it
// dispatches it, which it finds by far.String's "FAR(id=". If that format changes, the sink
// slows nothing and TestABatchIsTimedFromItsDeadline passes whatever the start.
type slowRuleLog struct{ delay time.Duration }

func (w slowRuleLog) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("FAR(id=")) {
		time.Sleep(w.delay)
	}

	return len(p), nil
}

// A batch is timed from where its deadline starts, before its rules are dispatched, so the
// histogram measures how close the batch came to the deadline it is read against. Timed from the
// join, a slow debug sink in the dispatch loop spent the deadline without being seen.
func TestABatchIsTimedFromItsDeadline(t *testing.T) {
	datapathWriteDuration.Reset()
	t.Cleanup(datapathWriteDuration.Reset)

	const delay = 50 * time.Millisecond
	saved := logger.BessLog
	logger.BessLog = zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(slowRuleLog{delay}), zapcore.DebugLevel)).Sugar()
	t.Cleanup(func() { logger.BessLog = saved })

	b := answeringBess(&pb.CommandResponse{}, nil)
	rules := PacketForwardingRules{fars: []far{{farID: 1, fseID: 0x2632898145f4d191, applyAction: ActionForward}}}
	b.SendMsgToUPF(upfMsgTypeMod, PacketForwardingRules{}, rules)

	var m dto.Metric
	if err := datapathWriteDuration.WithLabelValues(upfMsgTypeMod.String()).(prometheus.Histogram).Write(&m); err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(m.GetHistogram().GetSampleSum() * float64(time.Second)); got < delay {
		t.Errorf("the batch was timed at %s, less than the %s its dispatch took", got, delay)
	}
}
