// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package providers

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"io"
)

func RunDockerCommand(container string, cmd string) {
	inout := make(chan []byte)
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	waiter, err := cli.ContainerAttach(ctx, container, types.ContainerAttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})

	// Write to docker container
	go func(w io.WriteCloser) {
		for {
			data, ok := <-inout
			if !ok {
				w.Close()
				return
			}

			w.Write(append(data, '\n'))
		}
	}(waiter.Conn)
	inout <- []byte(cmd)

	waiter.Conn.Close()
	waiter.Close()
}
