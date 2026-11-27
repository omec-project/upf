// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// TestTheLocalDeleteKeepsWhatTheDatapathStillHolds pins the order of the local delete a
// Session Report Response with "context not found" triggers, on a UPF with no li block: the
// datapath delete runs first, and the session record and the UE address go only once it is
// answered. A refused delete keeps both, so the address is not reissued while a PDR still
// matches it; an accepted one releases both.
func TestTheLocalDeleteKeepsWhatTheDatapathStillHolds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refuse  bool
		wantErr bool
	}{
		{name: "refused", refuse: true, wantErr: true},
		{name: "accepted", refuse: false, wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const seid = 402

			pool, err := NewIPPool("10.250.0.0/24")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.LookupOrAllocIP(seid); err != nil {
				t.Fatal(err)
			}

			dp := newRefusingDP()
			dp.refuse = tc.refuse

			svc, err := sharedMetrics()
			if err != nil {
				t.Fatalf("metrics service: %v", err)
			}

			pConn := &PFCPConn{
				store:          NewInMemoryStore(),
				nodeID:         nodeID{remote: "1.1.1.1", local: "2.2.2.2", localIE: ie.NewNodeID("2.2.2.2", "", "")},
				upf:            &upf{datapath: dp, ippool: pool},
				InstrumentPFCP: svc,
			}
			sess := liStoredSession(seid, "10.250.0.14")
			sess.metrics = &metrics.Session{}
			if err = pConn.store.PutSession(sess); err != nil {
				t.Fatal(err)
			}

			err = pConn.handleSessionReportResponse(
				message.NewSessionReportResponse(0, 0, seid, 1, 0,
					ie.NewCause(ie.CauseSessionContextNotFound),
				))
			if (err != nil) != tc.wantErr {
				t.Fatalf("handleSessionReportResponse error = %v, want an error: %v", err, tc.wantErr)
			}

			_, stored := pConn.store.GetSession(seid)
			// Release reports whether the address was still held, so this asks the pool.
			held := pool.Release(seid)

			if stored != tc.refuse {
				t.Errorf("session stored = %v after a %s delete, want %v", stored, tc.name, tc.refuse)
			}
			if held != tc.refuse {
				t.Errorf("UE address held = %v after a %s delete, want %v", held, tc.name, tc.refuse)
			}
		})
	}
}
