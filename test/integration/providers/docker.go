// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package providers

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"
)

// MustRunDockerCommandAttach attaches to a running Docker container and executes a cmd.
// It should be used to spawn a new pfcpiface process inside and redirect its stdout/stderr to `docker logs`.
// This is equivalent to `docker attach` CLI command.
func MustRunDockerCommandAttach(container string, cmd string) {
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
	defer waiter.Close()
	if err = waiter.Conn.SetWriteDeadline(time.Now().Add(time.Second * 1)); err != nil {
		logrus.Fatalf("Failed to set deadline: %v", err)
	}
	if _, err = waiter.Conn.Write(append([]byte(cmd), '\n')); err != nil {
		logrus.Fatalf("Failed to write to container: %v", err)
	}
}

// RunDockerExecCommand executes a cmd inside a running Docker container.
// It should be used to invoke a "side" command inside a Docker container.
// This is equivalent to `docker exec` CLI command.
func RunDockerExecCommand(container string, cmd string) (
	code int, stdout string, stderr string, err error,
) {
	args := make([]string, 0)
	args = append(args, "exec", "-t", container)
	if strings.Contains(cmd, "/bin/sh") {
		// Just split in to "/bin/sh" "-c" and "actual_cmd"
		// This is useful for passing piped commands in to os/exec interface.
		args = append(args, strings.SplitN(cmd, " ", 3)...)
	} else {
		args = append(args, strings.Fields(cmd)...)
	}
	dockerCmd := exec.Command("docker", args...)
	stdoutPipe, err := dockerCmd.StdoutPipe()
	if err != nil {
		return 0, "", "", fmt.Errorf("error when connecting to stdout: %v", err)
	}
	stderrPipe, err := dockerCmd.StderrPipe()
	if err != nil {
		return 0, "", "", fmt.Errorf("error when connecting to stderr: %v", err)
	}
	if err := dockerCmd.Start(); err != nil {
		return 0, "", "", fmt.Errorf("error when starting command: %v", err)
	}

	stdoutBytes, _ := ioutil.ReadAll(stdoutPipe)
	stderrBytes, _ := ioutil.ReadAll(stderrPipe)

	if err := dockerCmd.Wait(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return e.ExitCode(), string(stdoutBytes), string(stderrBytes), nil
		}
		return 0, "", "", err
	}

	// command is successful
	return 0, string(stdoutBytes), string(stderrBytes), nil
}
