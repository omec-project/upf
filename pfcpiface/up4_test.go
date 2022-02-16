package pfcpiface

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_UP4_initTunnelPeerIDs(t *testing.T) {
	type args struct {
		mockUP4 *UP4
	}

	type want struct {
		length int
	}

	tests := []struct {
		name string
		args *args
		want *want
	}{
		{
			name: "init tunnelPeerIDPool",
			args: &args{
				mockUP4: &UP4{},
			},
			want: &want{
				length: maxGTPTunnelPeerIDs,
			},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				tt.args.mockUP4.initTunnelPeerIDs()
				require.Equal(t, tt.want.length, len(tt.args.mockUP4.tunnelPeerIDsPool))
			},
		)
	}
}

func Test_UP4_allocateGTPTunnelPeerID(t *testing.T) {
	type args struct {
		numAllocate int
		mockUP4     *UP4
	}

	type want struct {
		remainingInPool int
		startID         uint8
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{
			name: "allocate GTPTunnelPeerIDs",
			args: &args{
				mockUP4:     &UP4{},
				numAllocate: 2,
			},
			want: &want{
				remainingInPool: maxGTPTunnelPeerIDs - 2,
				startID:         2,
			},
		}, //TODO add drain test (allocate all tunnel peer IDs and expect error
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				tt.args.mockUP4.initTunnelPeerIDs()

				for i := 0; i < tt.args.numAllocate; i++ {
					got, err := tt.args.mockUP4.allocateGTPTunnelPeerID()
					if (err != nil) != tt.wantErr {
						t.Errorf("allocateGTPTunnelPeerID() error = %v, wantErr %v", err, tt.wantErr)
					}

					require.Equal(t, tt.want.startID+uint8(i), got)
				}

				require.Equal(t, tt.want.remainingInPool, len(tt.args.mockUP4.tunnelPeerIDsPool))

			},
		)
	}
}
