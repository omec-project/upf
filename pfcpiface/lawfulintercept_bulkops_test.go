// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// TestBulkDeactivationFollowsConfiguration drives the LI_T3 triggering interface the way its
// peer does — over mutual TLS, with a certificate the CC-TF's identity is bound into — and
// checks that the agreement stated in this UPF's JSON configuration is the one the element
// enforces.
//
// It goes through startTriggerListener rather than building an x1 server of its own,
// because a configured policy that never reaches the constructor is the failure this is for,
// and a second copy of the wiring in a test would not notice it.
//
// The message is hand-built: x1.Requester implements no bulk request, since no triggering
// function this project ships sends one. That is also what makes disabling both operations
// on a UPF free — see util's li/README.md.
func TestBulkDeactivationFollowsConfiguration(t *testing.T) {
	no := false

	cases := []struct {
		name        string
		configured  *bool
		wantRefusal bool
	}{
		{name: "agreed disabled", configured: &no, wantRefusal: true},
		{name: "no agreement in advance", configured: nil, wantRefusal: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

			cfg := &LiConfig{
				NEID:               testNEID,
				TFID:               testTFID,
				X1Listen:           freePort(t),
				DeactivateAllTasks: c.configured,
			}

			tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), nil, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
			if err != nil {
				t.Fatalf("startTriggerListener: %v", err)
			}

			// Tasking for the bulk request to act on, installed the way the CC-TF installs
			// it. Without it "the store is empty afterwards" would hold either way.
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
				CorrelationID: 0x2632898145f4d191,
				SEID:          14426627323429955319,
				SEIDAddress:   testLoopbackIP,
				DIDs:          []string{did},
			}); err != nil {
				t.Fatalf("ActivateTask: %v", err)
			}

			body := postX1(t, cfg.X1Listen, tfMat, bulkRequest("DeactivateAllTasksRequest", testTFID, testNEID))

			if !c.wantRefusal {
				if strings.Contains(body, "errorCode") {
					t.Fatalf("want the standard's default — bulk deactivation performed — got %s", body)
				}
				if tasks.Len() != 0 {
					t.Error("bulk deactivation was acknowledged and did not take effect")
				}

				return
			}

			if !strings.Contains(body, "<ns1:errorCode>5010</ns1:errorCode>") {
				t.Errorf("want the specification's 5010 for a disabled DeactivateAllTasks, got %s", body)
			}
			if want := "DeactivateAllTasks message is not enabled"; !strings.Contains(body, want) {
				t.Errorf("want the specification's own %q, got %s", want, body)
			}
			if tasks.Len() != 1 {
				t.Error("tasking was removed despite the refusal")
			}
		})
	}
}

// TestBulkRemovalFollowsConfiguration is the same assertion for the other switch, and it
// exists because without it the second of the two configuration fields is never observed
// to reach the element at all.
//
// The two are carried by one call taking both. So an element wired with the same field
// twice — the mistake that call is shaped to invite — reads the other field's unset value
// for bulk removal, performs the removal the operator disabled, and every other test here
// passes.
func TestBulkRemovalFollowsConfiguration(t *testing.T) {
	no := false

	cases := []struct {
		name        string
		configured  *bool
		wantRefusal bool
	}{
		{name: "agreed disabled", configured: &no, wantRefusal: true},
		{name: "no agreement in advance", configured: nil, wantRefusal: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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

			cfg := &LiConfig{
				NEID:                  testNEID,
				TFID:                  testTFID,
				X1Listen:              freePort(t),
				RemoveAllDestinations: c.configured,
			}

			if _, err := startTriggerListener(cfg, upfMat.ServerTLS(), nil, nil, x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil); err != nil {
				t.Fatalf("startTriggerListener: %v", err)
			}

			// A destination for the request to remove, provisioned the way the CC-TF
			// provisions one. The specification refuses the removal while a task still
			// references a destination, so no task is installed here.
			req := x1.NewRequester("https://"+cfg.X1Listen, testTFID, testNEID, tfMat.ClientTLS())
			if err := req.CreateDestination(x1.Destination{
				DID: testXIDTertiary, DeliveryType: testDeliveryX3Only,
				Address: testDeliveryIP, Port: 42069,
			}); err != nil {
				t.Fatalf("CreateDestination: %v", err)
			}

			body := postX1(t, cfg.X1Listen, tfMat, bulkRequest("RemoveAllDestinationsRequest", testTFID, testNEID))

			if !c.wantRefusal {
				if strings.Contains(body, "errorCode") {
					t.Fatalf("want the standard's default — bulk removal performed — got %s", body)
				}

				return
			}

			if !strings.Contains(body, "<ns1:errorCode>8020</ns1:errorCode>") {
				t.Errorf("want the specification's 8020 for a disabled RemoveAllDestinations, got %s", body)
			}
		})
	}
}

// TestBulkSwitchesInTheJSONConfig covers what the configuration layer does with the two
// keys, which is where an operator's `false` is at risk of quietly becoming nothing.
//
// The last two cases are the ones worth having. A key mistyped as a *value* must be
// refused rather than defaulted — the rule this config already applies to the fail-safe
// window, and for the same reason: defaulting here answers the permissive way. And the
// other fields of an `li` block are still mandatory, since these two are optional
// additions to a block that is not allowed to fail open.
func TestBulkSwitchesInTheJSONConfig(t *testing.T) {
	// The mandatory fields of an li block, with a placeholder for whatever the case adds.
	li := func(extra string) string {
		return `{
			"mode": "dpdk",
			"li": {
				"x3_sockaddr": "/pod-share/x3",
				"cert": "/li/upf.crt",
				"key": "/li/upf.key",
				"ca_cert": "/li/ca.crt",
				"x1_listen": ":8443",
				"tf_id": "smf-1",
				"ne_id": "upf-1"` + extra + `
			}
		}`
	}

	t.Run("both switches carry through", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li(`,
				"deactivate_all_tasks": false,
				"remove_all_destinations": true`), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.Li.DeactivateAllTasks == nil || *conf.Li.DeactivateAllTasks {
			t.Errorf("deactivate_all_tasks = %v, want a stated false — a dropped restriction "+
				"leaves the element performing the operation the operator withheld",
				conf.Li.DeactivateAllTasks)
		}
		if conf.Li.RemoveAllDestinations == nil || !*conf.Li.RemoveAllDestinations {
			t.Errorf("remove_all_destinations = %v, want a stated true", conf.Li.RemoveAllDestinations)
		}
	})

	t.Run("saying nothing is not saying false", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li(""), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.Li.DeactivateAllTasks != nil || conf.Li.RemoveAllDestinations != nil {
			t.Error("an li block that states no agreement must leave both unset, so the " +
				"standard's own defaults apply")
		}
	})

	// Refused, not defaulted — and refused on the LI object, where startLIShipper declines to
	// intercept and tells the ADMF, rather than by failing the load, which would stop the user
	// plane.
	t.Run("a value that is not a boolean is refused", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li(`,
				"deactivate_all_tasks": "false"`), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("a value of the wrong type in the li object failed the configuration "+
				"load, which stops the user plane: %v", err)
		}
		if conf.Li == nil || conf.Li.blockErr == nil {
			t.Error("a quoted \"false\" was accepted; an unparseable value must be refused " +
				"rather than read as unset, which is the permissive answer")
		}
	})

	// The mandatory fields are still mandatory — but they are enforced where a refusal
	// stops interception rather than the process. Loading the file must *succeed*: a
	// validateConf error reaches Fatalln and would take the user plane down over an LI
	// value, printing the field name into the general operator log. The refusal belongs
	// to the shipper, which has a fault channel and whose failure stops only itself.
	t.Run("the mandatory fields are still mandatory, and refused by the shipper", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(`{
			"mode": "dpdk",
			"li": {
				"x3_sockaddr": "/pod-share/x3",
				"cert": "/li/upf.crt",
				"key": "/li/upf.key",
				"ca_cert": "/li/ca.crt",
				"x1_listen": ":8443",
				"tf_id": "smf-1",
				"deactivate_all_tasks": false
			}
		}`, path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("loading a config whose li block is incomplete failed with %v; "+
				"an LI value must not be able to stop this network function", err)
		}
		if err := validateLiConfig(conf.Li); err == nil {
			t.Error("an li block with no ne_id was accepted; the optional switches must not " +
				"have made the rest of the block optional")
		}
	})
}

// postX1 sends one X1 request to addr as the CC-TF and returns the response body.
func postX1(t *testing.T, addr string, mat *mtls.Material, body []byte) string {
	t.Helper()

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: mat.ClientTLS()}}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://"+addr+"/X1/NE", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/xml")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post X1: %v", err)
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return string(out)
}

// bulkRequest builds one of the two bulk X1 messages: the one that stops every
// interception on this element, or the one that removes every destination it holds.
// The identifiers are parameters even though every caller passes the deployment's own:
// they are the two fields the element authorises the request against, and a builder that
// hard-coded them would be the wrong shape for the test that sends a wrong one.
//
//nolint:unparam // tfID is the field under authorisation; a constant builder would hide it
func bulkRequest(msgType, tfID, neID string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ns1:X1Request xmlns:ns1="http://uri.etsi.org/03221/X1/2017/10" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <ns1:x1RequestMessage xsi:type="ns1:` + msgType + `">
    <ns1:admfIdentifier>` + tfID + `</ns1:admfIdentifier>
    <ns1:neIdentifier>` + neID + `</ns1:neIdentifier>
    <ns1:messageTimestamp>2026-01-01T00:00:00.000000Z</ns1:messageTimestamp>
    <ns1:version>v1.6.1</ns1:version>
    <ns1:x1TransactionId>aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa</ns1:x1TransactionId>
  </ns1:x1RequestMessage>
</ns1:X1Request>`)
}

// TestLiBlockRefusesUnknownKeys covers the whole load path, not strictLiBlock alone: the property
// is that a mistyped LI key is refused, recorded for the LI subsystem, and does not fail the
// configuration load, and that is only true if LoadConfigFile calls the check.
//
// The two keys below are chosen because their defaults are not what the operator asked for and
// neither is mandatory, so validateLiConfig cannot see them go missing. A dropped
// `trigger_keepalive` leaves TIME_P2 at the clause default rather than the operator's. A dropped
// `admf_url` leaves the fault channel a no-op, so nothing
// this element is required to report — that one included — reaches the ADMF.
//
// The refusal stops interception, not the user plane: it is carried in blockErr, and the shipper
// declines to start on it and tells the ADMF.
func TestLiBlockRefusesUnknownKeys(t *testing.T) {
	li := func(extra string) string {
		return `{
			"mode": "dpdk",
			"li": {
				"x3_sockaddr": "/pod-share/x3",
				"cert": "/li/upf.crt",
				"key": "/li/upf.key",
				"ca_cert": "/li/ca.crt",
				"x1_listen": ":8443",
				"tf_id": "smf-1",
				"ne_id": "upf-1"` + extra + `
			}
		}`
	}

	t.Run("a conformant li block loads", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li(`,
				"trigger_keepalive": "5m",
				"admf_url": "https://admf:9443"`), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("a conformant li block was refused: %v", err)
		}
		if conf.Li.TriggerKeepalive != "5m" || conf.Li.AdmfURL != "https://admf:9443" {
			t.Errorf("the strict pass disturbed the decode: keepalive %q, admf %q",
				conf.Li.TriggerKeepalive, conf.Li.AdmfURL)
		}
		if conf.Li.blockErr != nil {
			t.Errorf("a conformant li block left a refusal recorded: %v", conf.Li.blockErr)
		}
	})

	for _, typo := range []string{"trigger_keepalve", "admf_uri"} {
		t.Run(typo, func(t *testing.T) {
			path := t.TempDir() + "/conf.jsonc"
			mustWriteStringToDisk(li(`,
				"`+typo+`": "5m"`), path)

			// **Both halves.** The key must be refused, *and* the user plane must still come
			// up: a configuration load that fails reaches cmd/pfcpiface's Fatalln, so a typo in
			// an optional subsystem would crash-loop the element carrying every subscriber's
			// traffic — and a user plane that will not start is the loudest way there is to
			// disclose that this element is LI-provisioned. Asserting only the refusal would
			// pass against exactly that.
			conf, err := LoadConfigFile(path)
			if err != nil {
				t.Fatalf("a misspelled LI key stopped the whole configuration load, which "+
					"crash-loops the user plane: %v", err)
			}
			if conf.Li == nil {
				t.Fatalf("the li object went missing; the refusal must be carried on it")
			}

			err = conf.Li.blockErr
			if err == nil {
				t.Fatalf("%q was accepted, so the setting the operator wrote never reached the "+
					"element and its unsafe default stands with nothing saying so", typo)
			}
			if !strings.Contains(err.Error(), typo) {
				t.Errorf("the refusal does not name the key that was wrong: %v", err)
			}
		})
	}

	t.Run("a key outside the li block is still tolerated", func(t *testing.T) {
		// The scope of the check, asserted rather than assumed. Configuration keys are added
		// over time; if strictness leaked past the li object, a configuration written for a newer
		// version would stop starting.
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(`{
			"mode": "dpdk",
			"a_key_this_version_does_not_model": 1,
			"li": {
				"x3_sockaddr": "/pod-share/x3",
				"cert": "/li/upf.crt",
				"key": "/li/upf.key",
				"ca_cert": "/li/ca.crt",
				"x1_listen": ":8443",
				"tf_id": "smf-1",
				"ne_id": "upf-1"
			}
		}`, path)

		if _, err := LoadConfigFile(path); err != nil {
			t.Fatalf("an unmodelled key outside the li block was refused, which would stop a "+
				"configuration written for a newer version from starting: %v", err)
		}
	})

	t.Run("no li block at all", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(`{"mode": "dpdk"}`, path)

		if _, err := LoadConfigFile(path); err != nil {
			t.Fatalf("a configuration without interception was refused: %v", err)
		}
	})
}

// TestLiBlockKeyIgnoresCase covers the spelling of the `li` key itself. encoding/json matches a
// key to a struct field ignoring case, so `"LI"` configured interception while the field was
// decoded through its tag. The strict decode reads the key out of the document by hand, and an
// exact lookup would leave such a configuration with interception off and nothing saying so.
func TestLiBlockKeyIgnoresCase(t *testing.T) {
	li := func(key, extra string) string {
		return `{
			"mode": "dpdk",
			"` + key + `": {
				"x3_sockaddr": "/pod-share/x3",
				"cert": "/li/upf.crt",
				"key": "/li/upf.key",
				"ca_cert": "/li/ca.crt",
				"x1_listen": ":8443",
				"tf_id": "smf-1",
				"ne_id": "upf-1"` + extra + `
			}
		}`
	}

	t.Run("LI configures interception", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li("LI", ""), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("an LI block was refused: %v", err)
		}
		if conf.Li == nil {
			t.Fatal("an LI block spelled in capitals left interception off with nothing saying so")
		}
		if conf.Li.blockErr != nil {
			t.Errorf("a conformant LI block left a refusal recorded: %v", conf.Li.blockErr)
		}
		if conf.Li.TFID != "smf-1" {
			t.Errorf("the LI block was not decoded: tf_id %q", conf.Li.TFID)
		}
	})

	t.Run("Li with a misspelled key is refused", func(t *testing.T) {
		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(li("Li", `,
				"admf_uri": "https://admf:9443"`), path)

		conf, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("a misspelled LI key stopped the whole configuration load: %v", err)
		}
		if conf.Li == nil || conf.Li.blockErr == nil {
			t.Fatal("a misspelled key in a Li block was not refused through blockErr")
		}
	})

	// An exact `li` beside another spelling is as ambiguous as two other spellings: taking the
	// exact one silently ignores the other, and an exact null beside an object would leave
	// interception off with nothing saying so.
	for name, doc := range map[string]string{
		"two spellings and no exact one": `{"mode": "dpdk", "LI": {"tf_id": "a"}, "Li": {"tf_id": "b"}}`,
		"an exact li beside another":     `{"mode": "dpdk", "li": {"tf_id": "a"}, "LI": {"tf_id": "b"}}`,
		"an exact null beside an object": `{"mode": "dpdk", "li": null, "LI": {"tf_id": "b"}}`,
	} {
		t.Run(name+" is refused", func(t *testing.T) {
			path := t.TempDir() + "/conf.jsonc"
			mustWriteStringToDisk(doc, path)

			conf, err := LoadConfigFile(path)
			if err != nil {
				t.Fatalf("an ambiguous li key stopped the whole configuration load: %v", err)
			}
			if conf.Li == nil || conf.Li.blockErr == nil {
				t.Fatal("two spellings of the li key were accepted, so one of them was picked silently")
			}
		})
	}
}
