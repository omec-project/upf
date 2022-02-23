// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_UP4_allocateGTPTunnelPeerID(t *testing.T) {
	type args struct {
		numAllocate int
		up4         *UP4
	}

	tests := []struct {
		name    string
		args    *args
		wantErr bool
	}{
		{
			name: "drain test allocateGTPTunnelPeerIDs",
			args: &args{
				up4:         &UP4{},
				numAllocate: maxGTPTunnelPeerIDs + 1,
			},
			wantErr: true,
		},
		{
			name: "test allocateGTPTunnelPeerIDs",
			args: &args{
				up4:         &UP4{},
				numAllocate: maxGTPTunnelPeerIDs,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				tt.args.up4.initTunnelPeerIDs()
				var err error

				for i := 0; i < tt.args.numAllocate; i++ {
					_, err = tt.args.up4.allocateGTPTunnelPeerID()
				}

				if tt.wantErr {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
			},
		)
	}
}
