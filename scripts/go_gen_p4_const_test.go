package main

import (
	"strconv"
	"strings"
	"testing"

	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	"github.com/stretchr/testify/require"
)

const dummyP4info = "dummy_p4info.txt"

func Test_generateConstants(t *testing.T) {
	type args struct {
		p4config *p4ConfigV1.P4Info
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
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   12345678,
				name: "TablePreQosPipeMyStation",
			},
		},
		{
			name: "verify action const",
			args: &args{
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   23766285,
				name: "ActionPreQosPipeInitializeMetadata",
			},
		},
		{
			name: "non existing const",
			args: &args{
				p4config: getP4Config(dummyP4info),
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
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   1024,
				name: "MeterSizePreQosPipeAppMeter",
			},
			wantErr: false,
		},
		{
			name: "verify dummy action",
			args: &args{
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   76544321,
				name: "MyDummyAction",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateConstants(tt.args.p4config)

			idx := strings.Index(result, tt.want.name)
			if idx == -1 && tt.wantErr {
				return
			}

			if idx != -1 && tt.wantErr {
				t.Fail()
			}

			line := strings.SplitN(result[idx:], "\n", 1)
			require.True(t, strings.Contains(strings.Join(line, " "), strconv.Itoa(tt.want.ID)))
		})
	}
}

func Test_generateTables(t *testing.T) {
	type args struct {
		p4config *p4ConfigV1.P4Info
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
			name: "verify table map",
			args: &args{
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   44976597,
				name: "PreQosPipe.sessions_uplink",
			},
		},
		{
			name: "non existing element",
			args: &args{
				p4config: getP4Config(dummyP4info),
			},
			want: &want{
				ID:   1111,
				name: "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateTables(tt.args.p4config)

			idx := strings.Index(result, tt.want.name)
			if idx == -1 && tt.wantErr {
				return
			}

			if idx == -1 && !tt.wantErr {
				// Avoid panics
				t.Fail()
			}

			line := strings.SplitN(result[idx:], ",", 1)
			require.True(t, strings.Contains(strings.Join(line, " "), strconv.Itoa(tt.want.ID)))
		})
	}
}
