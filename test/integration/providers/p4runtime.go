// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package providers

import (
	"context"
	"fmt"
	"github.com/antoninbas/p4runtime-go-client/pkg/client"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

var (
	stopCh   chan struct{}
	grpcConn *grpc.ClientConn
)

func TimeBasedElectionId() p4_v1.Uint128 {
	now := time.Now()
	return p4_v1.Uint128{
		High: uint64(now.Unix()),
		Low:  uint64(now.UnixNano() % 1e9),
	}
}

func ConnectP4rt(addr string, asMaster bool) (*client.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error

	grpcConn, err = grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	c := p4_v1.NewP4RuntimeClient(grpcConn)
	// Election only happens if asMaster is true.
	p4RtC := client.NewClient(c, 1, TimeBasedElectionId(), client.DisableCanonicalBytestrings)

	if asMaster {
		// perform Master Arbitration
		stopCh = make(chan struct{})
		arbitrationCh := make(chan bool)
		go p4RtC.Run(stopCh, arbitrationCh, nil)

		timeout := 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to connect to P4Runtime server")
		case <-arbitrationCh:
		}
	} else {
		// deletes channel, otherwise DisconnectP4rt blocks forever for non-master P4runtime channel
		stopCh = nil
	}

	// used to retrieve P4Info if exists on device
	p4RtC.GetFwdPipe(client.GetFwdPipeP4InfoAndCookie)

	return p4RtC, nil
}

func DisconnectP4rt() {
	if stopCh != nil {
		stopCh <- struct{}{}
	}
	// wait for P4rt stream to be closed
	// FIXME: p4runtime-go-client fatals if gRPC channel is closed before P4rt stream is terminated.
	//  The lib doesn't give a better way to wait for stream to be terminated.
	time.Sleep(1 * time.Second)
	if grpcConn != nil {
		grpcConn.Close()
	}
}
