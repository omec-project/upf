// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
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
