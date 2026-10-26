// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

func mustWriteStringToDisk(s string, path string) {
	err := os.WriteFile(path, []byte(s), 0o600)
	if err != nil {
		panic(err)
	}
}

func TestLoadConfigFile(t *testing.T) {
	t.Run("sample config is valid", func(t *testing.T) {
		s := `{
			"mode": "dpdk",
			"log_level": "info",
			"workers": 1,
			"max_sessions": 50000,
			"table_sizes": {
				"pdrLookup": 50000,
				"appQERLookup": 200000,
				"sessionQERLookup": 100000,
				"farLookup": 150000
			},
			"access": {
				"ifname": "access"
			},
			"core": {
				"ifname": "core"
			},
			"measure_upf": true,
			"measure_flow": true,
			"enable_notify_bess": true,
			"notify_sockaddr": "/pod-share/notifycp",
			"cpiface": {
				"dnn": "internet",
				"hostname": "upf",
				"http_port": "8080"
			},
			"n6_bps": 1000000000,
			"n6_burst_bytes": 12500000,
			"n3_bps": 1000000000,
			"n3_burst_bytes": 12500000,
			"qci_qos_config": [{
				"qci": 0,
				"cbs": 50000,
				"ebs": 50000,
				"pbs": 50000,
				"burst_duration_ms": 10,
				"priority": 7
			}]
		}`
		confPath := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(s, confPath)

		_, err := LoadConfigFile(confPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty config has log level info", func(t *testing.T) {
		s := `{
			"mode": "dpdk"
		}`
		confPath := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(s, confPath)

		conf, err := LoadConfigFile(confPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.LogLevel != zap.InfoLevel {
			t.Fatalf("expected log level %v, got %v", zap.InfoLevel, conf.LogLevel)
		}
	})

	t.Run("all sample configs must be valid", func(t *testing.T) {
		paths := []string{
			"../conf/upf.jsonc",
			"../ptf/config/upf.jsonc",
		}

		for _, path := range paths {
			_, err := LoadConfigFile(path)
			if err != nil {
				t.Errorf("config %v is not valid: %v", path, err)
			}
		}
	})
}

// TestRemoveCommentsKeepsStringContents pins the comment stripper to string context. A sweep for
// "//.*$" also matched the "//" inside a string value and deleted the rest of the line -- in a
// configuration rendered on one line, as the bess-upf chart renders it, the rest of the file -- so
// the parse failed as "unexpected end of JSON input", pointing nowhere near the value that caused
// it.
func TestRemoveCommentsKeepsStringContents(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url in a string is not a comment",
			in:   `{"url":"https://example.org:9443/path","hostname":"upf"}`,
			want: `{"url":"https://example.org:9443/path","hostname":"upf"}`,
		},
		{
			name: "line comment outside a string is removed",
			in:   "{\"a\":1} // trailing\n{\"b\":2}",
			want: "{\"a\":1} \n{\"b\":2}",
		},
		{
			name: "block comment outside a string is removed",
			in:   `{"a":/* note */1}`,
			want: `{"a":1}`,
		},
		{
			name: "comment markers inside a string survive",
			in:   `{"a":"/* not a comment */","b":"// nor this"}`,
			want: `{"a":"/* not a comment */","b":"// nor this"}`,
		},
		{
			name: "a block comment opened by /*/ is not closed by its own slash",
			in:   `{"b":2}/*/ x */`,
			want: `{"b":2}`,
		},
		{
			name: "block comment as the last bytes",
			in:   `{"a":1} /* end */`,
			want: `{"a":1} `,
		},
		{
			name: "block comment spanning lines",
			in:   "{\"c\":/* a\nb */3}",
			want: `{"c":3}`,
		},
		{
			name: "line comment at end of input with no newline",
			in:   `{"a":1} // end`,
			want: `{"a":1} `,
		},
		{
			name: "slash at end of input",
			in:   `{"a":1}/`,
			want: `{"a":1}/`,
		},
		{
			name: "unterminated block comment is left for the parser",
			in:   `{"a":1} /* trailing`,
			want: `{"a":1} /* trailing`,
		},
		{
			name: "escaped quote does not end the string",
			in:   `{"a":"say \"https://x\" ok"}`,
			want: `{"a":"say \"https://x\" ok"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := removeComments(tc.in); got != tc.want {
				t.Errorf("removeComments()\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestLoadConfigFileWithDoubleSlashInAString is the end-to-end form: a setting whose value
// holds "//" -- here a socket path with a doubled separator, which the kernel accepts -- must
// load as written, on one line or several.
func TestLoadConfigFileWithDoubleSlashInAString(t *testing.T) {
	for name, body := range map[string]string{
		"one line":      `{"mode":"af_packet","notify_sockaddr":"/pod-share//notifycp","cpiface":{"dnn":"internet"}}`,
		"several lines": "{\n  \"mode\": \"af_packet\",\n  \"notify_sockaddr\": \"/pod-share//notifycp\", // where the agent listens\n  \"cpiface\": {\"dnn\": \"internet\"}\n}",
	} {
		t.Run(name, func(t *testing.T) {
			confPath := t.TempDir() + "/conf.jsonc"
			mustWriteStringToDisk(body, confPath)

			conf, err := LoadConfigFile(confPath)
			if err != nil {
				t.Fatalf("a config holding \"//\" in a string did not load: %v", err)
			}

			if conf.NotifySockAddr != "/pod-share//notifycp" {
				t.Errorf("notify_sockaddr = %q, want %q", conf.NotifySockAddr, "/pod-share//notifycp")
			}

			if conf.CPIface.Dnn != "internet" {
				t.Errorf("cpiface.dnn = %q, want %q: the setting after the string was lost", conf.CPIface.Dnn, "internet")
			}
		})
	}
}

// An unterminated block comment is a broken file, and loading it must say so. Treating it as running
// to the end of input would load {"mode":"af_packet"} /* ... and silently drop whatever followed.
func TestLoadConfigFileRefusesAnUnterminatedBlockComment(t *testing.T) {
	confPath := t.TempDir() + "/conf.jsonc"
	mustWriteStringToDisk(`{"mode":"af_packet"} /* trailing`, confPath)

	if _, err := LoadConfigFile(confPath); err == nil {
		t.Error("a config ending in an unterminated block comment loaded")
	}
}
