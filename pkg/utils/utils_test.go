// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package utils

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"math"
	"net"
	"testing"
)

func TestInt2ip(t *testing.T) {
	tests := []struct {
		name string
		args uint32
		want net.IP
	}{
		{name: "zero", args: 0, want: net.IPv4zero.To4()},
		{name: "plain", args: 0x0a000001, want: net.ParseIP("10.0.0.1").To4()},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := Uint32ToIp4(tt.args)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func TestIp2int(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want uint32
	}{
		{name: "zero", ip: net.IPv4zero.To4(), want: 0},
		{name: "plain", ip: net.ParseIP("10.0.0.1").To4(), want: 0x0a000001},
		{name: "v6 mapped v4", ip: net.ParseIP("::ffff:10.0.0.1"), want: 0x0a000001},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := Ip4ToUint32(tt.ip); got != tt.want {
					t.Errorf("Ip4ToUint32() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func TestIp2int2IpTransitive(t *testing.T) {
	tests := []uint32{
		0,
		1,
		math.MaxUint32,
		0x0a000001,
	}
	for _, i := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ip := Uint32ToIp4(i)
			got := Ip4ToUint32(ip)
			require.Equal(t, i, got, "value %v failed transitive conversion with intermediate ip %v", ip)
		},
		)
	}
}
