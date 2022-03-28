// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"github.com/stretchr/testify/require"

	"net"
	"reflect"
	"testing"
)

func GetLoopbackInterface() (net.Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}

	for _, iface := range ifs {
		if (iface.Flags & net.FlagLoopback) != 0 {
			return iface, nil
		}
	}

	return net.Interface{}, ErrNotFound("No loopback interface found")
}

// This tests inherently depends on the host setup to a degree.
// If it's not feasible to run, we will skip it.
func TestGetUnicastAddressFromInterface(t *testing.T) {
	lb, err := GetLoopbackInterface()
	if err != nil {
		t.Skip("Skipping interface testing due to lack of suitable interfaces")
	}

	tests := []struct {
		name          string
		interfaceName string
		want          net.IP
		wantErr       bool
	}{
		{name: "loopback interface", interfaceName: lb.Name, want: net.ParseIP("127.0.0.1")},
		{name: "nonexistent interface", interfaceName: "invalid1234", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := GetUnicastAddressFromInterface(tt.interfaceName)
				if (err != nil) != tt.wantErr {
					t.Errorf(
						"GetUnicastAddressFromInterface() error = %v, wantErr %v", err, tt.wantErr,
					)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("GetUnicastAddressFromInterface() got = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func TestGetSliceTcMeterIndex(t *testing.T) {
	tests := []struct {
		name    string
		TC      uint8
		sliceID uint8
		want    int64
		wantErr bool
	}{
		{name: "SliceID=0, TC=0", sliceID: 0, TC: 0, want: 0},
		{name: "SliceID=3, TC=3", sliceID: 3, TC: 2, want: 14},
		{name: "SliceID=15, TC=3", sliceID: 15, TC: 3, want: 63},
		{name: "Big slice ID", sliceID: 16, TC: 3, wantErr: true},
		{name: "Big Traffic Class", sliceID: 0, TC: 4, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSliceTCMeterIndex(tt.sliceID, tt.TC)
			if (err != nil) != tt.wantErr {
				t.Errorf(
					"GetSliceTcMeterIndex() error = %v, wantErr %v", err, tt.wantErr,
				)
				return
			}
			require.Equal(t, tt.want, got)
		},
		)
	}
}
