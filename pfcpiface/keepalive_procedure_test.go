// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/x1"
)

// TestTheKeepaliveProcedureIsTheOperatorsOwn: the procedure of TS 103 221-1 clause 6.6.2
// runs by default on the clause's timers, keepalive_timeout is TIME_P2, and only
// keepalive_disabled turns it off. trigger_keepalive, the key's former name, still sets
// TIME_P2.
func TestTheKeepaliveProcedureIsTheOperatorsOwn(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  LiConfig
		want *x1.KeepaliveProcedure
	}{
		{"unset runs the clause defaults", LiConfig{}, &x1.KeepaliveProcedure{TriggeredOverLIT3: true}},
		{"disabled runs nothing", LiConfig{KeepaliveDisabled: true, KeepaliveTimeout: "5m"}, nil},
		{
			"keepalive_timeout is TIME_P2",
			LiConfig{KeepaliveTimeout: "5m"},
			&x1.KeepaliveProcedure{TimeP2: 5 * time.Minute, TriggeredOverLIT3: true},
		},
		{"every timer and the retain switch", LiConfig{
			KeepaliveTimeout: "30m", KeepaliveTimeP1: "2m",
			KeepaliveRetainTasking: true,
		}, &x1.KeepaliveProcedure{
			TimeP2: 30 * time.Minute, TimeP1: 2 * time.Minute,
			RetainTasking: true, TriggeredOverLIT3: true,
		}},
		{
			"trigger_keepalive is still TIME_P2",
			LiConfig{TriggerKeepalive: "5m"},
			&x1.KeepaliveProcedure{TimeP2: 5 * time.Minute, TriggeredOverLIT3: true},
		},
		{
			"both names agreeing",
			LiConfig{KeepaliveTimeout: "5m", TriggerKeepalive: "5m"},
			&x1.KeepaliveProcedure{TimeP2: 5 * time.Minute, TriggeredOverLIT3: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := keepaliveProcedure(&tc.cfg)
			if err != nil {
				t.Fatalf("keepaliveProcedure: %v", err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("keepaliveProcedure = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestTwoNamesForOneTimerMustAgree: keepalive_timeout and its deprecated name
// trigger_keepalive both set to different values is refused, naming both, because the
// element cannot know which the operator meant.
func TestTwoNamesForOneTimerMustAgree(t *testing.T) {
	_, err := keepaliveProcedure(&LiConfig{KeepaliveTimeout: "5m", TriggerKeepalive: "10m"})
	if err == nil {
		t.Fatal("two different values for TIME_P2 were accepted")
	}
	for _, name := range []string{"keepalive_timeout", "trigger_keepalive"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not name %s: %v", name, err)
		}
	}
}

// recordingADMF is an ADMF that records every report body it is sent and answers each with
// an empty 200 — not a bound X1 answer, so a report it receives counts as unanswered, which
// is the clause 6.6.2 case that deactivates tasking once TIME_P1 has passed. onReport, when
// set, is called with each body as it arrives. One built by newAnsweringADMF answers each
// report as a real ADMF does, with a bound OK.
type recordingADMF struct {
	url      string
	mu       sync.Mutex
	bodies   []string
	onReport func(body string)
	answers  bool
}

func newRecordingADMF(t *testing.T, onReport func(body string)) *recordingADMF {
	t.Helper()

	return startRecordingADMF(t, &recordingADMF{onReport: onReport})
}

// newAnsweringADMF is a recordingADMF that is alive and answers every report OK, as the real
// ADMF of a deployment does.
func newAnsweringADMF(t *testing.T) *recordingADMF {
	t.Helper()

	return startRecordingADMF(t, &recordingADMF{answers: true})
}

func startRecordingADMF(t *testing.T, a *recordingADMF) *recordingADMF {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test handler
		a.mu.Lock()
		a.bodies = append(a.bodies, string(body))
		a.mu.Unlock()
		if a.onReport != nil {
			a.onReport(string(body))
		}
		w.WriteHeader(http.StatusOK)
		if a.answers {
			_, _ = w.Write([]byte(admfOK(body))) //nolint:errcheck // test handler
		}
	}))
	t.Cleanup(srv.Close)
	a.url = srv.URL

	return a
}

// reporter is an X1 reporter sending to this ADMF, without TLS.
func (a *recordingADMF) reporter() *x1.Reporter {
	return x1.NewReporter(a.url, "admf", testNEID, nil)
}

// received reports whether any body sent so far contains want.
func (a *recordingADMF) received(want string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, b := range a.bodies {
		if strings.Contains(b, want) {
			return true
		}
	}

	return false
}

// TestATimeP3IsRefusedAtTheUPF: this element is triggered over LI_T3, where no answer to its
// keepalive report starts TIME_P3, so a configured keepalive_time_p3 would never run. It is
// refused rather than accepted and ignored.
func TestATimeP3IsRefusedAtTheUPF(t *testing.T) {
	if _, err := keepaliveProcedure(&LiConfig{KeepaliveTimeP3: "4h"}); err == nil {
		t.Error("keepalive_time_p3 was accepted at a POI triggered over LI_T3, where it never runs")
	}
}

// admfOK is the answer a live ADMF gives a report: the request's own response type, its
// identifiers, timestamp, version and transaction echoed, and an OK of
// AcknowledgedAndCompleted — which the reporter accepts as the report having arrived.
func admfOK(request []byte) string {
	var in struct {
		Messages []struct {
			Type            string `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
			AdmfIdentifier  string `xml:"admfIdentifier"`
			NeIdentifier    string `xml:"neIdentifier"`
			Timestamp       string `xml:"messageTimestamp"`
			Version         string `xml:"version"`
			X1TransactionID string `xml:"x1TransactionId"`
		} `xml:"x1RequestMessage"`
	}
	if err := xml.Unmarshal(request, &in); err != nil || len(in.Messages) != 1 {
		return ""
	}
	m := in.Messages[0]
	local := m.Type[strings.LastIndex(m.Type, ":")+1:]

	return `<?xml version="1.0"?><x1:X1Response xmlns:x1="http://uri.etsi.org/03221/X1/2017/10" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<x1:x1ResponseMessage xsi:type="x1:` + strings.TrimSuffix(local, "Request") + `Response">` +
		`<x1:admfIdentifier>` + m.AdmfIdentifier + `</x1:admfIdentifier>` +
		`<x1:neIdentifier>` + m.NeIdentifier + `</x1:neIdentifier>` +
		`<x1:messageTimestamp>` + m.Timestamp + `</x1:messageTimestamp>` +
		`<x1:version>` + m.Version + `</x1:version>` +
		`<x1:x1TransactionId>` + m.X1TransactionID + `</x1:x1TransactionId>` +
		`<x1:oK>AcknowledgedAndCompleted</x1:oK>` +
		`</x1:x1ResponseMessage></x1:X1Response>`
}
