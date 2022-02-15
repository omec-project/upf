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
)

//nolint:unused
func setupNewTranslator() *P4rtTranslator {
	p4infoBytes, _ := ioutil.ReadFile(p4InfoPath)

	var p4Config p4ConfigV1.P4Info

	_ = proto.UnmarshalText(string(p4infoBytes), &p4Config)

	return newP4RtTranslator(p4Config)
}

func Test_actionID(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "get NoAction",
			args:       "NoAction",
			translator: setupNewTranslator(),
			want:       uint32(21257015),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				//TODO add tests
			},
		)
	}
}

func Test_tableID(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "Existing table",
			args:       "PreQosPipe.Routing.routes_v4",
			translator: setupNewTranslator(),
			want:       uint32(39015874),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				// TODO add tests
			},
		)
	}
}

func Test_getCounterByName(t *testing.T) {
	type args struct {
		counterName string
		counterID   uint32
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
				counterID:   uint32(315693181),
				translator:  setupNewTranslator(),
			},
			want: &want{
				counterName: "PreQosPipe.pre_qos_counter",
				counterID:   uint32(315693181),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				//TODO add tests
			},
		)
	}
}

func Test_getTableByID(t *testing.T) {
	type args struct {
		tableID    uint32
		tableName  string
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
				tableID:    39015874,
				tableName:  "PreQosPipe.Routing.routes_v4",
				translator: setupNewTranslator(),
			},
			want: &want{
				tableID:   39015874,
				tableName: "PreQosPipe.Routing.routes_v4",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				// TODO add tests
			},
		)
	}
}
