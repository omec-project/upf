// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package providers

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"log"
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

// MustCreateNetworkIfNotExists tries to create a Docker network with 'name' (if not exists) and panics if it fails.
func MustCreateNetworkIfNotExists(name string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	allNetworks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		log.Fatalf("Failed to check if network exists: %v", err)
	}

	for _, net := range allNetworks {
		if net.Name == name {
			// network already exists
			return
		}
	}

	_, err = cli.NetworkCreate(ctx, name, types.NetworkCreate{})
	if err != nil {
		panic(err)
	}
}

// WaitForContainerRunning periodically (every 0.5 seconds) checks if a Docker container is in the 'Running' state.
// The function times out after 10 seconds.
func WaitForContainerRunning(name string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	timeout := time.After(10 * time.Second)
	ticker := time.Tick(500 * time.Millisecond)

	// Keep trying until we're timed out or get a result/error
	for {
		select {
		case <-timeout:
			return errors.New("timed out")
		case <-ticker:
			info, err := cli.ContainerInspect(ctx, name)
			if err != nil {
				return errors.New("failed to get container status")
			}

			if info.State.Running {
				return nil
			}
		}
	}
}

// MustRunDockerContainer is equivalent of 'docker run'.
// The function takes the following parameters:
// - name specifies a container name. The name is also used as a container hostname.
// - image specifies a name of Docker image to use.
// - cmd defines the initial command to pass to a container. It's optional and can be left empty.
// - exposedPorts specifies the list of L4 ports to expose. The format should be port_no/proto (e.g., 8080/tcp). It's optional.
// - mnt defines the mount paths. The format should be `<local_path>:<target_path>`. It's optional.
// - net defines a Docker network for a container (optional).
func MustRunDockerContainer(name, image, cmd string, exposedPorts []string, mnt string, net string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	baseCfg := &container.Config{
		Hostname:     name,
		Image:        image,
		OpenStdin:    true,
		Tty:          true,
		Cmd:          strings.Split(cmd, " "),
		ExposedPorts: nat.PortSet{},
		Volumes:      map[string]struct{}{},
	}

	hostCfg := &container.HostConfig{
		Privileged:   true,
		PortBindings: nat.PortMap{},
		Mounts:       []mount.Mount{},
	}

	if mnt != "" {
		mountPaths := strings.Split(mnt, ":")
		localPath := mountPaths[0]
		targetPath := mountPaths[1]

		baseCfg.Volumes[targetPath] = struct{}{}
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: localPath,
			Target: targetPath,
		})
	}

	for _, port := range exposedPorts {
		baseCfg.ExposedPorts[nat.Port(port)] = struct{}{}
		hostCfg.PortBindings[nat.Port(port)] = []nat.PortBinding{{
			HostIP:   "127.0.0.1",
			HostPort: port,
		}}
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{},
	}

	if net != "" {
		networkingConfig.EndpointsConfig[net] = &network.EndpointSettings{}
	}

	resp, err := cli.ContainerCreate(ctx, baseCfg, hostCfg, networkingConfig, nil, name)
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	if err := WaitForContainerRunning(resp.ID); err != nil {
		panic(err)
	}
}

// MustStopDockerContainer sends SIGKILL to the container process and removes a killed container.
func MustStopDockerContainer(name string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	err = cli.ContainerKill(ctx, name, "SIGKILL")
	if err != nil {
		logrus.Fatalf("Failed to stop Docker container %s: %v", name, err)
	}

	err = cli.ContainerRemove(ctx, name, types.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		logrus.Fatalf("Failed to stop Docker container %s: %v", name, err)
	}
}

func MustPullDockerImage(image string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	resp, err := cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}

	// waits for image to be pulled
	ioutil.ReadAll(resp)
}
