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

	// DefaultElectionID use reader election ID so that pfcpiface doesn't lose mastership.
	DefaultElectionID = p4_v1.Uint128{High: 0, Low: 1}
)

func ConnectP4rt(addr string, asMaster bool) (*client.Client, error) {
	grpcConn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	c := p4_v1.NewP4RuntimeClient(grpcConn)

	p4RtC := client.NewClient(c, 1, DefaultElectionID, client.DisableCanonicalBytestrings)

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
	if grpcConn != nil {
		grpcConn.Close()
	}
}
