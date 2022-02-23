// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_UP4_allocateAndReleaseGTPTunnelPeerID(t *testing.T) {
	type args struct {
		numAllocate        int
		up4                *UP4
		tunnelParams       []*tunnelParams
		releaseTunnelPeers bool
	}

	tests := []struct {
		name    string
		args    *args
		wantErr bool
	}{
		{
			name: "drain test allocateGTPTunnelPeerIDs",
			args: &args{
				up4:          &UP4{},
				numAllocate:  maxGTPTunnelPeerIDs + 1,
				tunnelParams: []*tunnelParams{},
			},
			wantErr: true,
		},
		{
			name: "test allocate/release GTPTunnelPeerIDs",
			args: &args{
				up4:                &UP4{},
				numAllocate:        maxGTPTunnelPeerIDs,
				releaseTunnelPeers: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				tt.args.up4.initTunnelPeerIDs()

				for i := 0; i < tt.args.numAllocate; i++ {
					got, err := tt.args.up4.allocateGTPTunnelPeerID()

					if tt.wantErr && i == tt.args.numAllocate-1 {
						// last cycle step. If we want error, now is the time
						require.Error(t, err)
						return
					}

					require.NoError(t, err)

					if tt.args.releaseTunnelPeers {
						tunnelParam := tunnelParams{
							tunnelIP4Src: ip2int(net.ParseIP("10.0.0.0")),
							tunnelIP4Dst: 0,
							tunnelPort:   uint16(i),
						}
						// FIXME releaseAllocatedGTPTunnelPeerID requires a tunnelParams object that is built in addOrUpdateGTPTunnelPeer.
						tt.args.up4.tunnelPeerIDs[tunnelParam] = got

						err = tt.args.up4.releaseAllocatedGTPTunnelPeerID(tunnelParam)
						require.NoError(t, err)
					}
				}
			},
		)
	}
}
