package integration

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"io"
	"testing"
	"time"
)

// FIXME: this test should be rewritten before merge
func TestBasicPFCPAssociation(t *testing.T) {
	inout := make(chan []byte)
	sendCommandToMockSMF := func (cmd string) {
		inout <- []byte(cmd)
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	waiter, err := cli.ContainerAttach(ctx, "mock-smf", types.ContainerAttachOptions{
		Stderr:       true,
		Stdout:       true,
		Stdin:        true,
		Stream:       true,
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

	sendCommandToMockSMF("associate")

	time.Sleep(time.Second*3)

	sendCommandToMockSMF("create --session-count 5 --base 0 --ue-pool 17.0.1.0/24 --enb-addr 140.0.101.1")

	// TODO: verify P4Runtime entries here

	time.Sleep(time.Second*10)
	sendCommandToMockSMF("stop")

	statusCh, errCh := cli.ContainerWait(ctx, "mock-smf", container.WaitConditionNotRunning)
	select {
	case <-errCh:
		cli.Close()
		t.Fail()
	case <-statusCh:
	}

	t.Fail()
}
