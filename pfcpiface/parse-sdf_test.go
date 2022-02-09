// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"net"
	"testing"
)

func mustParseCIDRNet(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		log.Fatal(err)
	}

	return ipNet
}

func Test_endpoint_parseNet(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    endpoint
		wantErr bool
	}{
		{name: "single IP",
			args:    "10.0.0.1",
			want:    endpoint{IPNet: mustParseCIDRNet("10.0.0.1/32")},
			wantErr: false},
		{name: "single IP with /32 net",
			args:    "10.0.0.1/32",
			want:    endpoint{IPNet: mustParseCIDRNet("10.0.0.1/32")},
			wantErr: false},
		{name: "single IP with net",
			args:    "10.0.0.1/24",
			want:    endpoint{IPNet: mustParseCIDRNet("10.0.0.1/24")},
			wantErr: false},
		{name: "single IPv6",
			args:    "2001:db8:a0b:12f0::1/32",
			want:    endpoint{IPNet: mustParseCIDRNet("2001:db8:a0b:12f0::1/32")},
			wantErr: false},
		{name: "invalid empty arg",
			args:    "",
			wantErr: true},
		{name: "malformed IP missing octet",
			args:    "10.0.1/24",
			wantErr: true},
		{name: "malformed IP",
			args:    "100",
			wantErr: true},
		{name: "malformed IP double slash",
			args:    "10.0.0.1/32/24",
			wantErr: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				var got endpoint
				if err := got.parseNet(tt.args); (err != nil) != tt.wantErr {
					t.Errorf("parseNet() error = %v, wantErr %v", err, tt.wantErr)
				}
				require.Equal(t, got, tt.want)
			},
		)
	}
}

func Test_endpoint_parsePort(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    endpoint
		wantErr bool
	}{
		{name: "single port",
			args:    "8080",
			want:    endpoint{ports: newExactMatchPortRange(8080)},
			wantErr: false},
		{name: "single port range",
			args:    "8080-8080",
			want:    endpoint{ports: newExactMatchPortRange(8080)},
			wantErr: false},
		{name: "normal port range",
			args:    "8080-8084",
			want:    endpoint{ports: newRangeMatchPortRange(8080, 8084)},
			wantErr: false},
		{name: "invalid empty port range",
			args:    "",
			wantErr: true},
		{name: "invalid inverted port range",
			args:    "100-90",
			wantErr: true},
		{name: "malformed double dash port range",
			args:    "100-200-300",
			wantErr: true},
		{name: "missing high port range",
			args:    "100-",
			wantErr: true},
		{name: "missing low port range",
			args:    "-100",
			wantErr: true},
		{name: "wrong separator",
			args:    "200,300",
			wantErr: true},
		{name: "malformed non-decimal number format",
			args:    "0x0000-0xffff",
			wantErr: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				var got endpoint
				if err := got.parsePort(tt.args); (err != nil) != tt.wantErr {
					t.Errorf("parsePort() error = %v, wantErr %v", err, tt.wantErr)
				}
				require.Equal(t, got, tt.want)
			},
		)
	}
}

func Test_ipFilterRule_String(t *testing.T) {
	type fields struct {
		action    string
		direction string
		proto     uint8
		src       endpoint
		dst       endpoint
	}

	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				ipf := &ipFilterRule{
					action:    tt.fields.action,
					direction: tt.fields.direction,
					proto:     tt.fields.proto,
					src:       tt.fields.src,
					dst:       tt.fields.dst,
				}
				require.Equal(t, ipf.String(), tt.want)
			},
		)
	}
}

func Test_newIpFilterRule(t *testing.T) {
	t.Run("new is wildcard filter", func(t *testing.T) {
		got := newIpFilterRule()
		if !got.dst.ports.isWildcardMatch() {
			t.Errorf("newIpFilterRule.dst.ports %v is not a wildcard", got)
		}
		if !got.src.ports.isWildcardMatch() {
			t.Errorf("newIpFilterRule.src.ports %v is not a wildcard", got)
		}
	})
}

func Test_parseAction(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{name: "permit action", args: "permit", wantErr: false},
		{name: "deny action", args: "deny", wantErr: false},
		{name: "empty action", args: "", wantErr: true},
		{name: "invalid action", args: "allow", wantErr: true},
		{name: "invalid action", args: "reject", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if err := parseAction(tt.args); (err != nil) != tt.wantErr {
					t.Errorf("parseAction() error = %v, wantErr %v", err, tt.wantErr)
				}
			},
		)
	}
}

func Test_parseDirection(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{name: "in direction", args: "in", wantErr: false},
		{name: "out direction", args: "out", wantErr: false},
		{name: "empty direction", args: "", wantErr: true},
		{name: "invalid direction", args: "both", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if err := parseDirection(tt.args); (err != nil) != tt.wantErr {
					t.Errorf("parseDirection() error = %v, wantErr %v", err, tt.wantErr)
				}
			},
		)
	}
}

func Test_parseFlowDesc(t *testing.T) {
	type args struct {
		flowDesc string
		ueIP     string
	}

	const ueIpString = "10.0.0.1"

	tests := []struct {
		name    string
		args    args
		want    *ipFilterRule
		wantErr bool
	}{
		{name: "empty flow description",
			args: args{flowDesc: "",
				ueIP: ""}, wantErr: true},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from any to assigned",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.102/32 to assigned",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from any to 60.60.0.102",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1/26 to 60.60.0.102",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1 8888 to 60.60.0.102/26",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1 8888-8888 to 60.60.0.102/26",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1 to 60.60.0.102 9999",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1 8888 to 60.60.0.102 9999",
				ueIP: ueIpString}, want: nil, wantErr: false},
		{name: "sample flow description",
			args: args{flowDesc: "permit out ip from 60.60.0.1 8888-8888 to 60.60.0.102 9999-9999",
				ueIP: ueIpString}, want: nil, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, err := parseFlowDesc(tt.args.flowDesc, tt.args.ueIP)
				if (err != nil) != tt.wantErr {
					t.Errorf("parseFlowDesc() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				// TODO: add wants and enable equal check
				// require.Equal(t, got, tt.want)
			},
		)
	}
}

func Test_parseL4Proto(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    uint8
		wantErr bool
	}{
		{name: "TCP proto", args: "tcp", want: 6, wantErr: false},
		{name: "UDP proto", args: "udp", want: 17, wantErr: false},
		{name: "numeric proto", args: "8", want: 8, wantErr: false},
		{name: "empty proto", args: "", want: 255, wantErr: true},
		{name: "hex proto", args: "0x10", want: 255, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := parseL4Proto(tt.args)
				if (err != nil) != tt.wantErr {
					t.Errorf("parseL4Proto() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				require.Equal(t, got, tt.want)
			},
		)
	}
}
