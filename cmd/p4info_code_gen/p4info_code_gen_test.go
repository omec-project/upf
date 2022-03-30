// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"io/fs"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	"github.com/stretchr/testify/require"
)

type generatorType int

const (
	constant generatorType = iota
	table
	action
	indirectCounter
	directCounter
	meter
)

const testP4InfoString = `
pkg_info {
  arch: "v1model"
}
tables {
  preamble {
    id: 12345678
    name: "PreQosPipe.my_station"
    alias: "my_station"
  }
  match_fields {
    id: 1
    name: "dst_mac"
    bitwidth: 48
    match_type: EXACT
  }
  action_refs {
    id: 21257015
  }
  size: 1024
}
actions {
  preamble {
    id: 26090030
    name: "PreQosPipe.set_source_iface"
    alias: "set_source_iface"
  }
  params {
    id: 1
    name: "src_iface"
    bitwidth: 8
  }
  params {
    id: 2
    name: "direction"
    bitwidth: 8
  }
  params {
    id: 3
    name: "slice_id"
    bitwidth: 4
  }
}
meters {
  preamble {
    id: 338231090
    name: "PreQosPipe.app_meter"
    alias: "app_meter"
  }
  spec {
    unit: BYTES
  }
  size: 1024
}
counters {
  preamble {
    id: 315693181
    name: "PreQosPipe.pre_qos_counter"
    alias: "pre_qos_counter"
  }
  spec {
    unit: BOTH
  }
  size: 1024
}
type_info {
	serializable_enums {
		key: "TrafficClass"
		value {
				underlying_type {
				bitwidth: 2
		  }
		  members {
				name: "BEST_EFFORT"
				value: "\000"
		  }
		  members {
				name: "CONTROL"
				value: "\001"
		  }
		  members {
				name: "REAL_TIME"
				value: "\002"
		  }
		  members {
				name: "ELASTIC"
				value: "\003"
		  }
		}
	}
	serializable_enums {
		key: "Fake"
		value {
				underlying_type {
				bitwidth: 48
		  }
		  members {
				name: "FAKE"
				value: "aaaaaa"
		  }
		}
	}
}
`

func mustWriteStringToDisk(s string, path string) {
	err := ioutil.WriteFile(path, []byte(s), fs.ModePerm)
	if err != nil {
		panic(err)
	}
}

func Test_generator(t *testing.T) {
	p4infoPath := t.TempDir() + "/dummy_p4info.pb.txt"
	mustWriteStringToDisk(testP4InfoString, p4infoPath)

	type args struct {
		p4config *p4ConfigV1.P4Info
		genType  generatorType
	}

	type want struct {
		ID   int
		name string
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{
			name: "verify table const",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   12345678,
				name: "TablePreQosPipeMyStation",
			},
		},
		{
			name: "verify action const",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   26090030,
				name: "ActionPreQosPipeSetSourceIface",
			},
		},
		{
			name: "non existing const",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   111111,
				name: "test",
			},
			wantErr: true,
		},
		{
			name: "verify meter size",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   1024,
				name: "MeterSizePreQosPipeAppMeter",
			},
		},
		{
			name: "verify table map",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  table,
			},
			want: &want{
				ID:   12345678,
				name: "PreQosPipe.my_station",
			},
		},
		{
			name: "non existing element",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  table,
			},
			want: &want{
				ID:   1111,
				name: "test",
			},
			wantErr: true,
		},
		{
			name: "verify meter map",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  meter,
			},
			want: &want{
				ID:   338231090,
				name: "PreQosPipe.app_meter",
			},
		},
		{
			name: "verify action map",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  action,
			},
			want: &want{
				ID:   26090030,
				name: "PreQosPipe.set_source_iface",
			},
		},
		{
			name: "non existing action",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  action,
			},
			want: &want{
				ID:   1,
				name: "test",
			},
			wantErr: true,
		},
		{
			name: "verify indirect counter map",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  indirectCounter,
			},
			want: &want{
				ID:   315693181,
				name: "PreQosPipe.pre_qos_counter",
			},
		},
		{
			name: "non existing indirect counter",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  indirectCounter,
			},
			want: &want{
				ID:   111,
				name: "test",
			},
			wantErr: true,
		},
		{
			name: "verify serializable enumerator",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   3,
				name: "EnumTrafficClassElastic",
			},
		},
		{
			name: "verify serializable enumerator non parsable",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   0,
				name: "EnumFakeFake",
			},
			wantErr: true,
		},
		{
			name: "verify match field bitwidth",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   48,
				name: "BitwidthMfDstMac",
			},
		},
		{
			name: "verify action parameter bitwidth",
			args: &args{
				p4config: mustGetP4Config(p4infoPath),
				genType:  constant,
			},
			want: &want{
				ID:   8,
				name: "BitwidthApSrcIface",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ""

			switch tt.args.genType {
			case constant:
				result = generateConstants(tt.args.p4config)
			case table:
				result = generateP4DataFunctions(tt.args.p4config, "Table")
			case action:
				result = generateP4DataFunctions(tt.args.p4config, "Action")
			case meter:
				result = generateP4DataFunctions(tt.args.p4config, "Meter")
			case indirectCounter:
				result = generateP4DataFunctions(tt.args.p4config, "Counter")
			case directCounter:
				result = generateP4DataFunctions(tt.args.p4config, "DirectCounter")
			}

			idx := strings.Index(result, tt.want.name)
			if idx == -1 && tt.wantErr {
				return
			}

			if idx != -1 && tt.wantErr {
				t.Fatalf("Found unexpected entity name %s in generated code %s", tt.want.name, result)
			}

			if idx == -1 {
				t.Fatalf("Did not find expected entity name '%s' in generated code: %s", tt.want.name, result)
			}

			line := strings.Join(strings.SplitN(result[idx:], "\n", 1), " ")
			require.Contains(t, line, strconv.Itoa(tt.want.ID), "ID not found")
		})
	}
}
