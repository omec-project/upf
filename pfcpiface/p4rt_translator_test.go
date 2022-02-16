// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"io/ioutil"
	"testing"

	//nolint:staticcheck // Ignore SA1019.
	// Upgrading to google.golang.org/protobuf/proto is not a drop-in replacement,
	// as also P4Runtime stubs are based on the deprecated proto.
	"github.com/golang/protobuf/proto"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	"github.com/stretchr/testify/require"
)

const (
	p4InfoTestPath = "../conf/p4/bin/p4info.txt"

	mockImmutableP4Info = "tables {\n  preamble {\n    id: 33586128\n    name: \"decap_cpu_header\"\n    alias: \"decap_cpu_header\"\n  }\n  action_refs {\n    id: 16788917\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33589124\n    name: \"forward\"\n    alias: \"forward\"\n  }\n  match_fields {\n    id: 1\n    name: \"routing_metadata.nhop_ipv4\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  action_refs {\n    id: 16780303\n  }\n  action_refs {\n    id: 16840314\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 512\n}\ntables {\n  preamble {\n    id: 33581985\n    name: \"ipv4_lpm\"\n    alias: \"ipv4_lpm\"\n  }\n  match_fields {\n    id: 1\n    name: \"ipv4.dstAddr\"\n    bitwidth: 32\n    match_type: LPM\n  }\n  action_refs {\n    id: 16812204\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33555613\n    name: \"send_arp_to_cpu\"\n    alias: \"send_arp_to_cpu\"\n  }\n  action_refs {\n    id: 16840314\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33562826\n    name: \"send_frame\"\n    alias: \"send_frame\"\n  }\n  match_fields {\n    id: 1\n    name: \"standard_metadata.egress_port\"\n    bitwidth: 9\n    match_type: EXACT\n  }\n  action_refs {\n    id: 16813016\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 256\n}\nactions {\n  preamble {\n    id: 16788917\n    name: \"do_decap_cpu_header\"\n    alias: \"do_decap_cpu_header\"\n  }\n}\nactions {\n  preamble {\n    id: 16780303\n    name: \"set_dmac\"\n    alias: \"set_dmac\"\n  }\n  params {\n    id: 1\n    name: \"dmac\"\n    bitwidth: 48\n  }\n}\nactions {\n  preamble {\n    id: 16840314\n    name: \"do_send_to_cpu\"\n    alias: \"do_send_to_cpu\"\n  }\n  params {\n    id: 1\n    name: \"reason\"\n    bitwidth: 16\n  }\n  params {\n    id: 2\n    name: \"cpu_port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 16784184\n    name: \"_drop\"\n    alias: \"_drop\"\n  }\n}\nactions {\n  preamble {\n    id: 16812204\n    name: \"set_nhop\"\n    alias: \"set_nhop\"\n  }\n  params {\n    id: 1\n    name: \"nhop_ipv4\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 16813016\n    name: \"rewrite_mac\"\n    alias: \"rewrite_mac\"\n  }\n  params {\n    id: 1\n    name: \"smac\"\n    bitwidth: 48\n  }\n}"
)

func getP4InfoFile(t *testing.T) *p4ConfigV1.P4Info {
	var p4Config p4ConfigV1.P4Info

	p4infoBytes, err := ioutil.ReadFile(p4InfoTestPath)
	require.NoError(t, err)

	err = proto.UnmarshalText(string(p4infoBytes), &p4Config)
	require.NoError(t, err)

	return &p4Config
}

func Test_actionID(t *testing.T) {

	var p4Config p4ConfigV1.P4Info
	_ = proto.UnmarshalText(mockImmutableP4Info, &p4Config)

	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "get _drop",
			args:       "_drop",
			translator: newP4RtTranslator(p4Config),
			want:       uint32(16784184),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := tt.translator.actionID(tt.args)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func Test_tableID(t *testing.T) {
	findTableIDFunc := func(p4Info p4ConfigV1.P4Info, name string) uint32 {
		for _, table := range p4Info.Tables {
			if table.GetPreamble().GetName() == name {
				return table.Preamble.Id
			}
		}

		return uint32(0)
	}

	p4Config := *getP4InfoFile(t)

	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "Existing table",
			args:       "PreQosPipe.Routing.routes_v4",
			translator: newP4RtTranslator(p4Config),
			want:       findTableIDFunc(p4Config, "PreQosPipe.Routing.routes_v4"),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := tt.translator.tableID(tt.args)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func Test_getCounterByName(t *testing.T) {
	findCounterIDFunc := func(p4Info p4ConfigV1.P4Info, name string) uint32 {
		for _, table := range p4Info.Counters {
			if table.GetPreamble().GetName() == name {
				return table.Preamble.Id
			}
		}

		return uint32(0)
	}

	p4Config := *getP4InfoFile(t)

	type args struct {
		counterName string
		translator  *P4rtTranslator
	}

	type want struct {
		counterName string
		counterID   uint32
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{name: "Existing counter",
			args: &args{
				counterName: "PreQosPipe.pre_qos_counter",
				translator:  newP4RtTranslator(p4Config),
			},
			want: &want{
				counterName: "PreQosPipe.pre_qos_counter",
				counterID:   findCounterIDFunc(p4Config, "PreQosPipe.pre_qos_counter"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.args.translator.getCounterByName(tt.args.counterName)
				if tt.wantErr {
					require.Error(t, err)
				}

				require.Equal(t, tt.want.counterID, got.GetPreamble().GetId())
			},
		)
	}
}

func Test_getTableByID(t *testing.T) {
	findTableIDFunc := func(p4Info p4ConfigV1.P4Info, name string) uint32 {
		for _, table := range p4Info.Tables {
			if table.GetPreamble().GetName() == name {
				return table.Preamble.Id
			}
		}

		return uint32(0)
	}

	p4Config := *getP4InfoFile(t)

	type args struct {
		tableID    uint32
		translator *P4rtTranslator
	}

	type want struct {
		tableID   uint32
		tableName string
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{name: "Existing table",
			args: &args{
				tableID:    findTableIDFunc(p4Config, "PreQosPipe.Routing.routes_v4"),
				translator: newP4RtTranslator(p4Config),
			},
			want: &want{
				tableID:   findTableIDFunc(p4Config, "PreQosPipe.Routing.routes_v4"),
				tableName: "PreQosPipe.Routing.routes_v4",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.args.translator.getTableByID(tt.args.tableID)
				if tt.wantErr {
					require.Error(t, err)
					return
				}

				require.Equal(t, tt.want.tableID, got.GetPreamble().GetId())
			},
		)
	}
}
