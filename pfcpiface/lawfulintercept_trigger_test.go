// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
	"github.com/wmnsk/go-pfcp/ie"
)

// liCA creates one throwaway LI certificate authority, writes its certificate to
// dir, and returns the path plus the material needed to issue leaves from it.
// Every certificate in a test must come from this one CA: a second CA would make
// the peers reject each other on chain validity, which looks nothing like the
// identity-binding refusals these tests are about.
func liCA(t *testing.T, dir string) (caPath string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()

	// ECDSA rather than RSA: an RSA-2048 key costs ~150 ms to generate under -race, and the LI
	// tests issue dozens of credentials per run.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}

	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test LI CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}

	caCert, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}

	caPath = dir + "/ca.crt"
	writePEM(t, caPath, &pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	return caPath, caCert, caKey
}

// liLeaf issues a certificate from ca binding identifier for the given X1 role.
// The binding is an annex G subjectAltName URN, one of the two forms clause 8.2.4
// accepts.
func liLeaf(t *testing.T, dir string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, role, identifier string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}

	binding, err := url.Parse("urn:etsi:li:103221-1:cert-binding:" + role + ":" + identifier)
	if err != nil {
		t.Fatalf("binding urn: %v", err)
	}

	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: identifier},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{identifier, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP(testLoopbackIP)},
		URIs:         []*url.URL{binding},
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}

	certPath = dir + "/" + identifier + ".crt"
	keyPath = dir + "/" + identifier + ".key"

	writePEM(t, certPath, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("leaf key encoding: %v", err)
	}
	writePEM(t, keyPath, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPath, keyPath
}

// writePEM writes PEM blocks to path with owner-only permissions, as LI
// credentials are.
func writePEM(t *testing.T, path string, blocks ...*pem.Block) {
	t.Helper()

	var out []byte
	for _, b := range blocks {
		out = append(out, pem.EncodeToMemory(b)...)
	}

	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// freeLocalAddr returns a loopback address on a port nothing is listening on, so the
// test does not collide with a fixed number.
func freeLocalAddr(t *testing.T) string {
	t.Helper()

	//nolint:noctx // test listener; no context needed
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := ln.Addr().String()
	_ = ln.Close()

	return addr
}

// TestTriggerListenerAcceptsCCTFTasking drives a real LI_T3 trigger over mutual
// TLS into the CC-POI's listener and checks the UPF ends up holding what it needs
// to attribute content: the warrant XID and the session's correlation identifier,
// findable by the F-SEID the datapath tags onto every duplicated packet.
func TestTriggerListenerAcceptsCCTFTasking(t *testing.T) {
	dir := t.TempDir()
	// Server credentials for the UPF, client credentials for the SMF's CC-TF. The
	// CC-TF presents the "ADMF" role: on this interface it is the tasking party.
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	cfg := &LiConfig{
		NEID:     testNEID,
		TFID:     testTFID,
		X1Listen: freeLocalAddr(t),
	}

	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), nil, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}

	const seid = 14426627323429955319

	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	// The destination has to exist before a trigger may name it: this POI refuses a
	// content trigger whose destinations it does not know, so that a triggering
	// function whose provisioning has been lost finds out.
	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}

	trigger := x1.Trigger{
		XID:           testXIDPrimary,
		ProductID:     testXIDSecondary,
		CorrelationID: 0x2632898145f4d191,
		SEID:          seid,
		SEIDAddress:   testLoopbackIP,
		DIDs:          []string{testXIDTertiary},
	}

	if err := req.ActivateTask(trigger); err != nil {
		t.Fatalf("ActivateTask: %v", err)
	}

	// The same trigger naming a destination this POI has never heard of must be
	// refused rather than acknowledged. Accepting it would duplicate a subject's
	// traffic and discard every copy while the triggering function believed
	// interception was running — which is what a POI restart would otherwise cause.
	unknown := trigger
	unknown.XID = "99999999-9999-4999-8999-999999999999"
	unknown.SEID = seid + 7
	unknown.DIDs = []string{"55555555-5555-4555-8555-555555555555"}
	if err := req.ActivateTask(unknown); err == nil {
		t.Error("a content trigger naming an unknown destination was accepted")
	}
	if _, _, ok := lookupTrigger(tasks, nil, seid+7); ok {
		t.Error("a refused trigger was installed anyway")
	}

	task, _, ok := lookupTrigger(tasks, nil, seid)
	if !ok {
		t.Fatal("no trigger found for the tasked session")
	}

	if task.DeliveryXID() != trigger.ProductID {
		t.Errorf("DeliveryXID() = %q, want the warrant XID %q", task.DeliveryXID(), trigger.ProductID)
	}

	if task.CorrelationID != trigger.CorrelationID {
		t.Errorf("CorrelationID = %d, want %d", task.CorrelationID, trigger.CorrelationID)
	}

	// A session nobody tasked must not resolve to a warrant, or content would be
	// delivered labelled with someone else's.
	if _, _, ok := lookupTrigger(tasks, nil, seid+1); ok {
		t.Error("an untasked session resolved to a trigger")
	}

	if err := req.DeactivateTask(trigger.XID); err != nil {
		t.Fatalf("DeactivateTask: %v", err)
	}

	if _, _, ok := lookupTrigger(tasks, nil, seid); ok {
		t.Error("trigger still installed after DeactivateTask")
	}
}

// TestTriggerListenerRejectsForeignTasker aims the same attack at this interface:
// a certificate the LI CA legitimately issued, asserting an identity it is not
// bound to. Being inside the LI trust domain is not authority to task this UPF.
func TestTriggerListenerRejectsForeignTasker(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	// A valid LI-CA certificate, but bound to a different element's identity.
	otherCert, otherKey := liLeaf(t, dir, caCert, caKey, "ADMF", "smf-2")

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t)}

	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), nil, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	otherMat, err := mtls.Load(otherCert, otherKey, caPath)
	if err != nil {
		t.Fatalf("load other material: %v", err)
	}

	const seid = 99

	// Assert the authorised triggering function's identifier while holding a
	// certificate bound to smf-2.
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, otherMat.ClientTLS())
	err = req.ActivateTask(x1.Trigger{
		XID:           testXIDPrimary,
		ProductID:     testXIDSecondary,
		CorrelationID: 1,
		SEID:          seid,
		DIDs:          []string{testXIDTertiary},
	})
	if err == nil {
		t.Fatal("a certificate bound to another element was allowed to task this UPF")
	}

	var reqErr *x1.RequestError
	if e, ok := err.(*x1.RequestError); ok {
		reqErr = e
	}

	if reqErr == nil || reqErr.Code != 1030 {
		t.Errorf("err = %v, want X1 error 1030 (identifier does not match certificate)", err)
	}

	if _, _, ok := lookupTrigger(tasks, nil, seid); ok {
		t.Error("a refused trigger was installed anyway")
	}
}

// TestTriggerListenerBindFailureIsReported checks the fail-closed behaviour: a
// CC-POI that cannot be tasked must not come up looking healthy, and the fault
// goes to the ADMF rather than a general log.
func TestTriggerListenerBindFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	// Occupy the port so the listener cannot bind.
	//nolint:noctx // test listener; no context needed
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = busy.Close() }()

	rec := &recordingReporter{}
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: busy.Addr().String()}

	if _, err := startTriggerListener(cfg, upfMat.ServerTLS(), rec, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil); err == nil {
		t.Fatal("startTriggerListener reported success on a port it could not bind")
	}

	if len(rec.reported()) != 1 || rec.reported()[0] != x1.NEIssueX1ListenFailed {
		t.Errorf("reported issues = %v, want one %s", rec.reported(), x1.NEIssueX1ListenFailed)
	}
}

// TestShipDropsContentWithoutATask is the guard at the point of delivery.
// Duplication (PFCP DUPL) and tasking (LI_T3) arrive over different interfaces
// and can disagree, so content whose session has no task must be dropped and
// reported — never shipped with an XID no mediation function can attribute.
func TestShipDropsContentWithoutATask(t *testing.T) {
	const seid = 42

	// [fseid(8)][action(1)][inner IP packet]
	tagged := make([]byte, 0, 9+20)
	tagged = binary.LittleEndian.AppendUint64(tagged, seid)
	tagged = append(tagged, farForwardUAndDuplicate)
	tagged = append(tagged, 0x45, 0x00, 0x00, 0x14, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2)

	newShipper := func(rec *recordingReporter) *liShipper {
		return &liShipper{
			tasks:    store.New(),
			reporter: rec,
			senders:  make(map[string]x2x3.Sender),
			// A shipper never runs without one — startLIShipper refuses to start
			// without an element identifier — and the framing happens before the
			// per-destination sender lookup, since one PDU is built and shared.
			ids: x2x3.NewIdentity(testNEID, upfInterceptionPoint),
		}
	}

	t.Run("untasked session", func(t *testing.T) {
		rec := &recordingReporter{}
		s := newShipper(rec)

		s.ship(tagged)

		if len(s.senders) != 0 {
			t.Error("a delivery client was created for content with no interception task")
		}

		if len(rec.reported()) != 1 || rec.reported()[0] != x1.NEIssueContentUntasked {
			t.Errorf("reported = %v, want one %s", rec.reported(), x1.NEIssueContentUntasked)
		}
	})

	t.Run("tasked but no X3 destination", func(t *testing.T) {
		rec := &recordingReporter{}
		s := newShipper(rec)
		// A task whose only destination is for signalling: content must not be sent
		// to an X2 endpoint.
		s.tasks.Activate(types.InterceptTask{
			XID:           testXIDPrimary,
			ProductID:     testXIDSecondary,
			CorrelationID: 7,
			Targets:       []types.TargetIdentifier{{Type: types.TargetFSEID, Value: "42"}},
			Products:      []types.ProductType{types.ProductCC},
			Deliveries:    []types.DeliveryEndpoint{{Type: types.DeliveryX2, Address: testX3SockAddr}},
		})

		s.ship(tagged)

		if len(s.senders) != 0 {
			t.Error("content was prepared for delivery with no X3 destination")
		}

		if len(rec.reported()) != 1 || rec.reported()[0] != x1.NEIssueInvalidConfig {
			t.Errorf("reported = %v, want one %s", rec.reported(), x1.NEIssueInvalidConfig)
		}
	})

	t.Run("no delivery credentials", func(t *testing.T) {
		rec := &recordingReporter{}
		s := newShipper(rec)
		s.tasks.Activate(types.InterceptTask{
			XID:           testXIDPrimary,
			ProductID:     testXIDSecondary,
			CorrelationID: 7,
			Targets:       []types.TargetIdentifier{{Type: types.TargetFSEID, Value: "42"}},
			Products:      []types.ProductType{types.ProductCC},
			Deliveries:    []types.DeliveryEndpoint{{Type: types.DeliveryX3, Address: testX3SockAddr}},
		})

		s.ship(tagged)

		// Intercept product is never delivered over an unauthenticated connection,
		// so with no LI credentials loaded there is no sender and the fault is
		// reported instead.
		if len(s.senders) != 0 {
			t.Error("a delivery client was created without LI credentials")
		}

		if len(rec.reported()) != 1 || rec.reported()[0] != x1.NEIssueMDFUnreachable {
			t.Errorf("reported = %v, want one %s", rec.reported(), x1.NEIssueMDFUnreachable)
		}
	})
}

// recordingReporter captures the NE issues raised, so a test can assert what the
// ADMF would have been told. Guarded, because reports also arrive from background
// goroutines — the keepalive fail-safe raises one from its own watchdog.
type recordingReporter struct {
	mu           sync.Mutex
	issues       []string
	descriptions []string
}

func (r *recordingReporter) Notify(issueType, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.issues = append(r.issues, issueType)
	r.descriptions = append(r.descriptions, description)
}

// NotifyAsync records synchronously. The double is not where the non-blocking
// property lives — that belongs to x1.Reporter and is asserted against the real one —
// so recording here keeps every existing assertion about *what* was reported exact,
// and keeps a test from having to wait for a goroutine to say so.
func (r *recordingReporter) NotifyAsync(issueType, description string) {
	r.Notify(issueType, description)
}

// reported returns a copy of what has been raised so far.
func (r *recordingReporter) reported() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.issues...)
}

// described returns the descriptions that went with them, in the same order — for
// the faults whose usefulness is in what they name rather than in the type alone.
func (r *recordingReporter) described() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.descriptions...)
}

// TestTriggerKeepaliveFailSafePurgesTasking: tasking must not
// outlive the party responsible for it. A triggering function that restarts
// forgets the triggers it installed, and content intercepted under a trigger
// nobody can withdraw keeps flowing past the point where the warrant itself is
// revoked. The fail-safe makes that lapse instead.
//
// The two halves run on separate listeners, side by side: one triggering function keeps
// talking past the point where silence would have purged it and then goes quiet, so the
// keepalives are shown to restart the fail-safe rather than disarm it; the other is quiet
// from the start.
func TestTriggerKeepaliveFailSafePurgesTasking(t *testing.T) {
	t.Parallel()

	const seid = 4242

	t.Run("keepalives hold the tasking until they stop", func(t *testing.T) {
		t.Parallel()

		tasks, req, admf := keepaliveTasked(t, seid)

		// A keepalive keeps it alive: the fail-safe must not remove tasking merely
		// because no new session happened to be established. Kept up past TIME_P2 plus
		// TIME_P1 from the activation (2 s), when silence would already have purged it, so
		// a keepalive that did not restart TIME_P2 is caught here rather than passing.
		for range 4 {
			time.Sleep(600 * time.Millisecond)

			if err := req.Keepalive(); err != nil {
				t.Fatalf("Keepalive: %v", err)
			}
		}

		if _, _, ok := lookupTrigger(tasks, nil, seid); !ok {
			t.Fatal("tasking was purged while the triggering function was still talking")
		}

		// And then stops, as a triggering function that restarts does. The keepalives
		// restarted the fail-safe; they must not have disarmed it.
		if !awaitPurge(tasks, seid, 10*time.Second) {
			t.Error("tasking outlived a triggering function that had kept it alive and then went " +
				"quiet: the keepalives disarmed the fail-safe instead of restarting it")
		}
		awaitTold(t, admf, x1.NEIssueKeepalivesMissing, x1.NEIssueTaskingPurged)
	})

	t.Run("silence purges the tasking and tells the ADMF", func(t *testing.T) {
		t.Parallel()

		tasks, _, admf := keepaliveTasked(t, seid)

		// The triggering function goes quiet, as a restarted one does.
		if !awaitPurge(tasks, seid, 10*time.Second) {
			t.Error("tasking outlived the triggering function; content would keep being intercepted with nobody able to withdraw it")
		}

		// And the ADMF is told, because interception stopping must not be silent: first that
		// keepalives stopped (fault 9050), then that the tasking was deactivated (Alert 10000).
		awaitTold(t, admf, x1.NEIssueKeepalivesMissing, x1.NEIssueTaskingPurged)
	})

	t.Run("an ADMF answering the report does not keep an orphaned trigger alive", func(t *testing.T) {
		t.Parallel()

		// The real ADMF is alive and answers the 9050 report OK. Over LI_T3 the triggering
		// function, not this ADMF, plays the ADMF (TS 33.128 clause 5.2.6): its answer says
		// nothing about the function that installed the trigger, so it must not start the two
		// hours of TIME_P3, which would leave an orphan intercepting for that long.
		tasks, _, admf := keepaliveTaskedReportingTo(t, seid, newAnsweringADMF(t))

		if !awaitPurge(tasks, seid, 10*time.Second) {
			t.Error("an ADMF's OK kept a trigger alive after the triggering function went quiet: " +
				"content keeps being intercepted for a warrant no live process holds")
		}
		awaitTold(t, admf, x1.NEIssueKeepalivesMissing, x1.NEIssueTaskingPurged)
	})
}

// awaitPurge reports whether the trigger for seid is gone from tasks within limit.
func awaitPurge(tasks *store.Store, seid uint64, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, _, ok := lookupTrigger(tasks, nil, seid); !ok {
			return true
		}

		time.Sleep(25 * time.Millisecond)
	}

	_, _, ok := lookupTrigger(tasks, nil, seid)
	return !ok
}

// awaitTold fails t unless admf receives each of conditions, each within five seconds.
func awaitTold(t *testing.T, admf *recordingADMF, conditions ...string) {
	t.Helper()

	for _, condition := range conditions {
		deadline := time.Now().Add(5 * time.Second)
		for !admf.received(condition) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if !admf.received(condition) {
			t.Errorf("the ADMF was not told %s", condition)
		}
	}
}

// keepaliveTasked starts a trigger listener running the keepalive procedure at its shortest
// timers, has a triggering function install one trigger for seid on it, and returns the
// task store, that triggering function's requester and the ADMF the element reports to.
func keepaliveTasked(t *testing.T, seid uint64) (*store.Store, *x1.Requester, *recordingADMF) {
	t.Helper()

	return keepaliveTaskedReportingTo(t, seid, newRecordingADMF(t, nil))
}

// keepaliveTaskedReportingTo is keepaliveTasked with the ADMF the element reports to given.
func keepaliveTaskedReportingTo(t *testing.T, seid uint64, admf *recordingADMF) (*store.Store, *x1.Requester, *recordingADMF) {
	t.Helper()

	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	cfg := &LiConfig{
		NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t),
		// The shortest the procedure allows. TIME_P1 is the wait after the 9050 report, which
		// no answer ends at a POI triggered over LI_T3.
		KeepaliveTimeout: "1s", KeepaliveTimeP1: "1s",
	}

	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), admf.reporter(), nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}

	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}

	if err := req.ActivateTask(x1.Trigger{
		XID:           testXIDPrimary,
		ProductID:     testXIDSecondary,
		CorrelationID: 7,
		SEID:          seid,
		DIDs:          []string{did},
	}); err != nil {
		t.Fatalf("ActivateTask: %v", err)
	}

	if _, _, ok := lookupTrigger(tasks, nil, seid); !ok {
		t.Fatal("trigger was not installed")
	}

	return tasks, req, admf
}

// TestTheStopReportFollowsTheStop pins the ordering obligation the asynchronous
// re-derivation created: the element reports that content interception has ceased,
// and that report has to follow the datapath actually having been programmed to stop
// — not merely a re-derivation having been requested.
//
// Worth a test of its own because the property is held by one call. The hook waits
// for the pass it asked for; swap that back to the fire-and-forget form and every
// other test in this package still passes while the report becomes a claim about
// something that has not happened yet.
//
// Driven through the keepalive fail-safe because that is the removal path that
// reports, and it exercises the same hook an ordinary withdrawal does.
func TestTheStopReportFollowsTheStop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	var mu sync.Mutex
	var events []string
	note := func(e string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}

	// A session the trigger's criterion selects, so re-deriving actually programs
	// something and the ordering has two events to be an ordering between.
	const seid = 4242
	sessions := NewInMemoryStore()
	if putErr := sessions.PutSession(unmarkedSession(seid, testUEIPv4)); putErr != nil {
		t.Fatalf("PutSession: %v", putErr)
	}

	// The push that stops duplication is held until the test releases it, so a report sent
	// without waiting for the stop would arrive while the datapath is still duplicating. Without
	// the hold the push completes long before the report reaches the ADMF over HTTPS, and the
	// order below would hold whether or not the purge waited for it.
	stopping := make(chan struct{})
	release := make(chan struct{})
	var holdOnce sync.Once
	enabler := newCCEnabler(nil, confirmedPush(func(_, updated PacketForwardingRules) uint8 {
		for i := range updated.fars {
			if updated.fars[i].Duplicates() {
				note("program:on")

				continue
			}
			holdOnce.Do(func() {
				close(stopping)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
				}
			})
			note("program:off")
		}

		return ie.CauseRequestAccepted
	}), nil)
	t.Cleanup(enabler.stop)
	enabler.addSource(sessions)

	admf := newRecordingADMF(t, func(body string) {
		if strings.Contains(body, x1.NEIssueTaskingPurged) {
			note("report:" + x1.NEIssueTaskingPurged)
		}
	})
	cfg := &LiConfig{
		NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t),
		KeepaliveTimeout: "1s", KeepaliveTimeP1: "1s",
	}
	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), admf.reporter(),
		enabler, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	if err := req.ActivateTask(x1.Trigger{
		XID:           testXIDPrimary,
		ProductID:     testXIDSecondary,
		CorrelationID: 7,
		SEID:          seid,
		DIDs:          []string{did},
	}); err != nil {
		t.Fatalf("ActivateTask: %v", err)
	}

	// Then the triggering function goes quiet and the fail-safe reclaims the tasking.
	select {
	case <-stopping:
	case <-time.After(15 * time.Second):
		t.Fatal("the fail-safe never began stopping the duplication")
	}
	if _, _, ok := lookupTrigger(tasks, nil, seid); ok {
		t.Fatal("tasking outlived the triggering function")
	}

	// While the stop is held, the purge must not have been reported: the report says
	// interception has stopped, and it has not.
	for heldUntil := time.Now().Add(750 * time.Millisecond); time.Now().Before(heldUntil); {
		if admf.received(x1.NEIssueTaskingPurged) {
			close(release)
			t.Fatal("the purge was reported while the datapath was still being told to stop " +
				"duplicating; the report describes a state that does not hold yet")
		}
		time.Sleep(25 * time.Millisecond)
	}
	close(release)
	// The alert is sent after the purge; wait for it to arrive before reading the order.
	for alertBy := time.Now().Add(5 * time.Second); !admf.received(x1.NEIssueTaskingPurged) && time.Now().Before(alertBy); {
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	stopped, reported := -1, -1
	for i, e := range events {
		if e == "program:off" && stopped < 0 {
			stopped = i
		}
		if e == "report:"+x1.NEIssueTaskingPurged && reported < 0 {
			reported = i
		}
	}

	if stopped < 0 {
		t.Fatalf("the datapath was never programmed to stop duplicating: %v", events)
	}
	if reported < 0 {
		t.Fatalf("interception stopping was not reported: %v", events)
	}
	if stopped > reported {
		t.Errorf("the stop was reported before the datapath was programmed to stop (%v); "+
			"the report describes a state that does not hold yet", events)
	}
}

// TestOverlappingWarrantsPickTheSameOneEveryTime: when two warrants cover one
// session this POI delivers each packet under exactly one of them, so which one
// has to be the same on every packet. Selecting from a map's iteration order —
// which is what reading store.Match unsorted amounted to — scattered a session's
// packets across the covering warrants at random, leaving each agency with an
// arbitrary fraction of the content and none of them with a usable stream. The
// agencies that end up with nothing are told as a task fault against each of them,
// which TestAWarrantServedNothingByAnOverlapSaysSo covers.
func TestOverlappingWarrantsPickTheSameOneEveryTime(t *testing.T) {
	const seid = 42

	target := types.TargetIdentifier{Type: types.TargetFSEID, Value: "42"}
	tasks := store.New()
	for _, xid := range []types.XID{
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	} {
		tasks.Activate(types.InterceptTask{
			XID: xid, ProductID: xid, CorrelationID: 7, Targets: []types.TargetIdentifier{target},
			Products:   []types.ProductType{types.ProductCC},
			Deliveries: []types.DeliveryEndpoint{{Type: types.DeliveryX3, Address: testX3SockAddr}},
		})
	}

	const wantXID = types.XID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	for i := range 50 {
		task, _, ok := lookupTrigger(tasks, nil, seid)
		if !ok {
			t.Fatalf("pass %d: tasked session did not resolve to a trigger", i)
		}
		if task.XID != wantXID {
			t.Fatalf("pass %d: selected warrant %q, want %q on every packet", i, task.XID, wantXID)
		}
	}
}

// TestTriggerKeepaliveMustBeValid: an unparseable keepalive timer is checked before the
// X1 listener serves, so the error returned here cannot leave an element accepting and
// applying tasking into a store its caller has abandoned — un-tasked to its operator,
// holding warrants in fact.
func TestTriggerKeepaliveMustBeValid(t *testing.T) {
	for _, v := range []string{testUnparseable, "-5m", "0s"} {
		if _, err := parseKeepaliveTimer("keepalive_timeout", v); err == nil {
			t.Errorf("parseKeepaliveTimer(%q) accepted an unusable timer", v)
		}
	}
	if d, err := parseKeepaliveTimer("keepalive_timeout", ""); err != nil || d != 0 {
		t.Errorf(`parseKeepaliveTimer("") = %v, %v; want 0, nil (the clause default)`, d, err)
	}
	if d, err := parseKeepaliveTimer("keepalive_timeout", "5m"); err != nil || d != 5*time.Minute {
		t.Errorf(`parseKeepaliveTimer("5m") = %v, %v; want 5m, nil`, d, err)
	}

	// Nothing may be left listening when the timer is rejected.
	addr := freeLocalAddr(t)
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: addr, KeepaliveTimeout: testUnparseable}
	if _, err := startTriggerListener(cfg, &tls.Config{}, nil, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil); err == nil {
		t.Fatal("startTriggerListener accepted an invalid keepalive_timeout")
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("X1 port still bound after a rejected keepalive_timeout: %v", err)
	}
	_ = ln.Close()
}

// TestOrdinaryWithdrawalIsNotReportedAsAPurge: only the fail-safe purge — "the
// triggering function went quiet" — is reported as one, not the withdrawals the
// triggering function itself ordered, which happen on every session end. The
// channel that says a controlling function has
// stopped answering is the channel the durability of withdrawal depends on, and an
// operator who sees it on every normal deactivation learns to ignore it.
func TestOrdinaryWithdrawalIsNotReportedAsAPurge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	admf := newRecordingADMF(t, nil)
	// The default keepalive procedure, whose TIME_P2 is an hour: nothing here is meant to
	// lapse, so anything reported as a purge would be this element mislabelling a
	// withdrawal it was asked for.
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t)}

	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), admf.reporter(), nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}

	const seid, xid = 4242, types.XID(testXIDPrimary)
	const did = testXIDTertiary
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	if err := req.ActivateTask(x1.Trigger{
		XID:           xid,
		ProductID:     testXIDSecondary,
		CorrelationID: 7,
		SEID:          seid,
		DIDs:          []string{did},
	}); err != nil {
		t.Fatalf("ActivateTask: %v", err)
	}
	if _, _, ok := lookupTrigger(tasks, nil, seid); !ok {
		t.Fatal("trigger was not installed")
	}

	if err := req.DeactivateTask(xid); err != nil {
		t.Fatalf("DeactivateTask: %v", err)
	}

	// The withdrawal itself must still have happened — the acknowledgement above is
	// what says so, and this is what it says about.
	if _, _, ok := lookupTrigger(tasks, nil, seid); ok {
		t.Error("the task survived its own deactivation")
	}
	// Long enough for an asynchronous report to have arrived, had one been sent.
	time.Sleep(300 * time.Millisecond)
	if admf.received(x1.NEIssueTaskingPurged) {
		t.Error("an explicit DeactivateTask was reported as a fail-safe purge")
	}
}

// correlationBytes is the correlation value as it reaches the numbering context: the
// same big-endian encoding shipperPDU puts in x2x3.PDU.CorrelationID. Written once so a
// test cannot assert against a context the element never numbers under.
func correlationBytes(v uint64) [x2x3.CorrelationIDLength]byte {
	var corr [x2x3.CorrelationIDLength]byte
	binary.BigEndian.PutUint64(corr[:], v)

	return corr
}

// TestNumberingIsReleasedOnEveryKindOfRemoval: the sequence-numbering state belongs
// to the tasking that created it, not to the circumstances of its removal. This
// element holds one context per intercepted session, so a warrant that outlives
// many sessions would otherwise leave one behind for each — and it is dropped on an
// ordinary withdrawal and a bulk deactivation alike, even though only the fail-safe
// purge is reported.
func TestNumberingIsReleasedOnEveryKindOfRemoval(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t)}
	if _, startErr := startTriggerListener(cfg, upfMat.ServerTLS(), &recordingReporter{}, nil, ids, nil, nil); startErr != nil {
		t.Fatalf("startTriggerListener: %v", startErr)
	}

	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}

	// **One ProductID for both.** Two different ones are the one arrangement in which
	// releasing numbering by XID cannot go wrong, so they would hide that defect: this
	// element's triggering function allocates a task per (warrant, session, UPF) and
	// gives them all the warrant's ProductID, so two live tasks sharing a delivery XID
	// is the *ordinary* case here rather than an exotic one. With two XIDs, releasing
	// numbering by XID happens to be correct.
	type tasking struct{ trigger, warrant types.XID }
	const oneWarrant = types.XID("aaaaaaaa-1111-4111-8111-111111111111")
	withdrawn := tasking{testXIDPrimary, oneWarrant}
	bulked := tasking{testXIDSecondary, oneWarrant}

	for i, task := range []tasking{withdrawn, bulked} {
		if err := req.ActivateTask(x1.Trigger{
			XID:           task.trigger,
			ProductID:     task.warrant,
			CorrelationID: uint64(i + 1),
			SEID:          uint64(4242 + i),
			DIDs:          []string{did},
		}); err != nil {
			t.Fatalf("ActivateTask: %v", err)
		}
		// Number one PDU under each, as the shipper does when it delivers content: that
		// is what creates the state this test is about.
		//
		// The correlation bytes are built the way the shipper builds them — big-endian,
		// as x2x3.PDU.CorrelationID is filled — not as a byte pattern of this test's own.
		// A hand-made pattern is a context the element never numbers under, so the
		// release would be asserted against state nothing produced.
		ids.Attributes(task.warrant.Bytes(), correlationBytes(uint64(i+1)), time.Now(), nil, nil)
	}
	if n := ids.Contexts(); n != 2 {
		t.Fatalf("numbering contexts = %d, want 2 — the state this test tracks was never created", n)
	}

	if err := req.DeactivateTask(withdrawn.trigger); err != nil {
		t.Fatalf("DeactivateTask: %v", err)
	}
	if n := ids.Contexts(); n != 1 {
		t.Errorf("numbering contexts = %d after an ordinary withdrawal, want 1 (the other task's): "+
			"the two share a delivery XID, so releasing by XID takes the surviving session's "+
			"numbering with it", n)
	}

	if body := postX1(t, cfg.X1Listen, tfMat, bulkRequest("DeactivateAllTasksRequest", testTFID, testNEID)); strings.Contains(body, "errorCode") {
		t.Fatalf("bulk deactivation refused: %s", body)
	}
	if n := ids.Contexts(); n != 0 {
		t.Errorf("numbering contexts = %d after a bulk deactivation, want 0 — numbering "+
			"outlived the tasking it belongs to", n)
	}
	// The fail-safe purge needs no case of its own, and that is a property of the library
	// rather than an omission here: x1.purgeAllTasking is the one implementation behind
	// both the bulk deactivation above and the keepalive lapse, differing only in the
	// PurgeReason it reports, and it calls notifyRemoved per task — so the release is per
	// task in both. What could differ is exactly what this checks: that releasing
	// per context still empties the state when every task goes.
}

// nextSequenceNumber is the number the identity would put on the next PDU of a
// context, read from the attributes it builds. It advances the context by one, which is
// what a delivered PDU does — so a test asserts on the number rather than on how many
// contexts exist.
//
// Reading the number is the point. A count of contexts passes against a release that
// dropped the wrong entry and recreated it on the next PDU, which is precisely the
// defect's shape: the surviving context looks present and starts again from zero.
// The label is a parameter because the numbering is keyed by it: the relabel tests pass a
// superseded one and the current one to the same helper, and which they pass is the property.
//
//nolint:unparam // xid is the numbering key under test, not incidental
func nextSequenceNumber(t *testing.T, ids *x2x3.Identity, xid types.XID, corr uint64) uint32 {
	t.Helper()

	for _, a := range ids.Attributes(xid.Bytes(), correlationBytes(corr), time.Now(), nil, nil) {
		if a.Type == x2x3.AttrSequenceNumber && len(a.Value) == 4 {
			return binary.BigEndian.Uint32(a.Value)
		}
	}
	t.Fatal("the identity built no sequence number attribute")

	return 0
}

// TestEndingOneSessionDoesNotRenumberAnother is the numbering-integrity property at the
// element, in the arrangement this element actually runs in.
//
// The CC Triggering Function allocates one task per (warrant, session, UPF) and gives
// them all the warrant's ProductID, which is what goes on the wire as the XID. So one
// delivery XID is shared by every session that warrant is intercepting here, and
// releasing the numbering state *by XID* when one session ends restarts the numbering of
// all the others.
//
// That is not a leak, it is a forgery. A sequence number is how a mediation function
// detects loss on this interface, so numbering that resets under a live context makes
// this element emit a sequence the receiver must read as duplication or as a gap — for a
// warrant whose product is otherwise entirely correct. Under a target with two
// concurrent PDU sessions, or under ULCL where a branching point and a session anchor
// both serve one session, this is the ordinary case.
func TestEndingOneSessionDoesNotRenumberAnother(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}
	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}

	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t)}
	if _, err := startTriggerListener(cfg, upfMat.ServerTLS(), &recordingReporter{}, nil, ids, nil, nil); err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}

	// One warrant, two of its PDU sessions at this UPF. Same ProductID, different
	// correlation values and different detection criteria — exactly what the triggering
	// function sends.
	const warrant = types.XID("aaaaaaaa-1111-4111-8111-111111111111")
	sessionA := struct {
		trigger types.XID
		corr    uint64
		seid    uint64
	}{testXIDPrimary, 0x2632898145f4d191, 4242}
	sessionB := struct {
		trigger types.XID
		corr    uint64
		seid    uint64
	}{testXIDSecondary, 0x7ab3120945f4d192, 4243}

	for _, s := range []struct {
		trigger types.XID
		corr    uint64
		seid    uint64
	}{sessionA, sessionB} {
		if err := req.ActivateTask(x1.Trigger{
			XID:           s.trigger,
			ProductID:     warrant,
			CorrelationID: s.corr,
			SEID:          s.seid,
			DIDs:          []string{did},
		}); err != nil {
			t.Fatalf("ActivateTask: %v", err)
		}
	}

	// Both sessions deliver content: three copies of A, three of B. The next number in
	// each context is therefore 3.
	for range 3 {
		nextSequenceNumber(t, ids, warrant, sessionA.corr)
		nextSequenceNumber(t, ids, warrant, sessionB.corr)
	}

	// Session A ends — an ordinary PDU-session release, which the triggering function
	// turns into a DeactivateTask for that session's trigger alone. Session B is
	// untouched and its interception is still live.
	if err := req.DeactivateTask(sessionA.trigger); err != nil {
		t.Fatalf("DeactivateTask: %v", err)
	}

	if got := nextSequenceNumber(t, ids, warrant, sessionB.corr); got != 3 {
		t.Errorf("the surviving session's next xCC is numbered %d, want 3: ending one of this "+
			"warrant's sessions restarted the numbering of another one it is still intercepting, "+
			"so the mediation function receives duplicated sequence numbers under a live context "+
			"— which is how it decides whether it has everything", got)
	}

	// And the ended session's context is gone: a new task under the same warrant and the
	// same correlation value is a new context and starts at zero.
	if got := nextSequenceNumber(t, ids, warrant, sessionA.corr); got != 0 {
		t.Errorf("the released context resumed at %d, want 0", got)
	}
}

// TestARelabelReleasesTheSupersededLabelsContextOnly is the modification side of the
// same granularity question, and it has both halves because each is a way of getting it
// wrong.
//
// A relabel moves the delivery XID, which is what the numbering is keyed by — so the
// context under the superseded label is stranded, and released. But the triggering
// function relabels a warrant's triggers one at a time, so while one is being modified
// its sibling sessions are still delivering under the old label: releasing by XID would
// take their numbering with it. And a modification that does not touch the labelling must
// release nothing at all, because those contexts are the ones this task's own copies are
// still using.
func TestARelabelReleasesTheSupersededLabelsContextOnly(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	tfCert, tfKey := liLeaf(t, dir, caCert, caKey, "ADMF", testTFID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}
	tfMat, err := mtls.Load(tfCert, tfKey, caPath)
	if err != nil {
		t.Fatalf("load tf material: %v", err)
	}

	ids := x2x3.NewIdentity(testNEID, upfInterceptionPoint)
	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freeLocalAddr(t)}
	if _, err := startTriggerListener(cfg, upfMat.ServerTLS(), &recordingReporter{}, nil, ids, nil, nil); err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}
	req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())

	const did = testXIDTertiary
	if err := req.CreateDestination(x1.Destination{
		DID: did, DeliveryType: testDeliveryX3Only, Address: testDeliveryIP, Port: 42069,
	}); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}

	const (
		oldLabel = types.XID("aaaaaaaa-1111-4111-8111-111111111111")
		newLabel = types.XID("cccccccc-3333-4333-8333-333333333333")
		triggerA = types.XID(testXIDPrimary)
		triggerB = types.XID(testXIDSecondary)
	)
	const corrA, corrB = uint64(0x2632898145f4d191), uint64(0x7ab3120945f4d192)

	for _, tr := range []x1.Trigger{
		{XID: triggerA, ProductID: oldLabel, CorrelationID: corrA, SEID: 4242, DIDs: []string{did}},
		{XID: triggerB, ProductID: oldLabel, CorrelationID: corrB, SEID: 4243, DIDs: []string{did}},
	} {
		if err := req.ActivateTask(tr); err != nil {
			t.Fatalf("ActivateTask: %v", err)
		}
	}

	// Both sessions deliver, so both contexts exist under the old label.
	for range 3 {
		nextSequenceNumber(t, ids, oldLabel, corrA)
		nextSequenceNumber(t, ids, oldLabel, corrB)
	}

	// A modification that changes nothing the numbering depends on: same label. Nothing
	// may be released, or this task's own next copy repeats a number.
	if err := req.ModifyTask(x1.Trigger{
		XID: triggerA, ProductID: oldLabel, CorrelationID: corrA, SEID: 4242, DIDs: []string{did},
	}); err != nil {
		t.Fatalf("ModifyTask (no relabel): %v", err)
	}
	if got := nextSequenceNumber(t, ids, oldLabel, corrA); got != 3 {
		t.Fatalf("a modification that did not move the label renumbered the context to %d, want 3: "+
			"the next copy repeats a sequence number the mediation function has already seen, which "+
			"is how it detects loss", got)
	}

	// Now the relabel of session A alone, which is how the triggering function
	// propagates one: session B is still delivering under the old label.
	if err := req.ModifyTask(x1.Trigger{
		XID: triggerA, ProductID: newLabel, CorrelationID: corrA, SEID: 4242, DIDs: []string{did},
	}); err != nil {
		t.Fatalf("ModifyTask (relabel): %v", err)
	}

	if got := nextSequenceNumber(t, ids, oldLabel, corrB); got != 3 {
		t.Errorf("a sibling session still delivering under the old label was renumbered to %d, "+
			"want 3: relabelling one of a warrant's triggers released the numbering of the ones "+
			"the ADMF has not relabelled yet", got)
	}
	// The superseded context is gone rather than held for the life of the process.
	if got := nextSequenceNumber(t, ids, oldLabel, corrA); got != 0 {
		t.Errorf("the superseded label's context resumed at %d, want 0 — it was left behind, and "+
			"nothing will ever number under it again", got)
	}
}

// TestTriggerKeepaliveHasAFloor: a parseable but far-too-short TIME_P2 is the more
// damaging configuration mistake, because the element starts and looks healthy.
//
// A triggering function's keepalive cadence is not configurable at the triggering
// function, so a window shorter than a couple of them purges tasking that is live
// and being answered for. The fail-safe then fires as a fault rather than as a
// backstop, and the report it raises names the triggering function as the thing
// that went silent — sending an operator to investigate an element that was
// behaving correctly, while interception the agency believes is running has
// stopped.
func TestTriggerKeepaliveHasAFloor(t *testing.T) {
	for _, tc := range []struct {
		window string
		reject bool
	}{
		{"", false},      // the clause default, an hour
		{"5m", false},    // the documented example
		{"2m30s", false}, // exactly the floor
		{"1s", true},     // shorter than a single keepalive cadence
		{testKeepaliveP1, true},
		{"1m", true}, // one cadence: any jitter reads as absence
	} {
		d, err := parseKeepaliveTimer("keepalive_timeout", tc.window)
		if err != nil {
			t.Fatalf("parseKeepaliveTimer(%q): %v", tc.window, err)
		}
		if got := tooShortTriggerKeepalive(d); got != tc.reject {
			t.Errorf("tooShortTriggerKeepalive(%q) = %v, want %v", tc.window, got, tc.reject)
		}
	}
}

// TestASenderNoTriggerReferencesIsClosed is the reclamation half of the delivery pool.
//
// A delivery client is created on first use, so without reclamation destination churn — a
// warrant relabelled to a new MDF3, an agency's endpoint moved, a triggering function
// re-provisioning after its own restart — would accumulate a delivery worker, a TLS
// connection and a keepalive timer per address for the life of the process. Worse than a
// leak: this element keepalives each of them, and a keepalive is
// indistinguishable at the far end from an element that is still delivering, so it would hold off
// the fail-safe of a mediation function it has nothing left to send.
func TestASenderNoTriggerReferencesIsClosed(t *testing.T) {
	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)
	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	tasks := store.New()
	s := &liShipper{
		tasks:     tasks,
		tlsConfig: upfMat.ClientTLS(),
		senders:   make(map[string]x2x3.Sender),
		ids:       x2x3.NewIdentity(testNEID, upfInterceptionPoint),
		keepalive: x2x3.KeepaliveConfig{Disabled: true},
	}

	const (
		oldAddr = "192.0.2.1:42069"
		newAddr = "192.0.2.9:42069"
	)
	warrant := types.InterceptTask{
		XID:      testXIDPrimary,
		Products: []types.ProductType{types.ProductCC},
		DIDs:     []string{testXIDTertiary},
		Deliveries: []types.DeliveryEndpoint{
			{DID: testXIDTertiary, Type: types.DeliveryX3, Address: oldAddr},
		},
	}
	if !tasks.Activate(warrant) {
		t.Fatal("Activate failed")
	}

	// Content goes to the destination the trigger names, which is what creates the sender.
	if _, err := s.senderFor(oldAddr); err != nil {
		t.Fatalf("senderFor: %v", err)
	}
	if n := len(s.senders); n != 1 {
		t.Fatalf("holding %d senders, want 1", n)
	}

	// The warrant is relabelled to a different endpoint — an ordinary ModifyTask, and the
	// event that makes the old address unreferenced.
	relabelled := warrant
	relabelled.Deliveries = []types.DeliveryEndpoint{
		{DID: testXIDTertiary, Type: types.DeliveryX3, Address: newAddr},
	}
	if !tasks.Activate(relabelled) {
		t.Fatal("re-activating the relabelled task failed")
	}
	if _, err := s.senderFor(newAddr); err != nil {
		t.Fatalf("senderFor: %v", err)
	}

	s.reclaimUnreferencedSenders()

	s.mu.Lock()
	_, oldHeld := s.senders[oldAddr]
	_, newHeld := s.senders[newAddr]
	held := len(s.senders)
	s.mu.Unlock()

	if oldHeld {
		t.Error("the delivery client for an endpoint no trigger names is still held: its worker " +
			"runs and its keepalives keep reaching a mediation function this element has nothing " +
			"left to send, which holds off that peer's own fail-safe")
	}
	if !newHeld {
		t.Error("the delivery client for the endpoint the trigger now names was reclaimed")
	}
	if held != 1 {
		t.Errorf("holding %d senders after reclamation, want 1", held)
	}

	// And the teardown takes the rest.
	s.close()

	s.mu.Lock()
	remaining := len(s.senders)
	s.mu.Unlock()

	if remaining != 0 {
		t.Errorf("holding %d senders after teardown, want 0: a reconfiguration or a shutdown left "+
			"workers running and connections open with nothing left to send them", remaining)
	}
}
