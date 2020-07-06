package main

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("usage: rebuild-container [containerID]\n")
		fmt.Printf("Destroys and rebuilds a container with the same configuration.\n")
		os.Exit(1)
	}

	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintf(os.Stderr, err.(error).Error())
			os.Exit(1)
		}
	}()

	containerID := os.Args[1]

	docker, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	container, err := docker.ContainerInspect(context.Background(), containerID)
	if err != nil {
		panic(err)
	}

	// TODO: Should store the container's config somewhere, in case of failure between the remove and the
	// start
	err = docker.ContainerRemove(context.Background(), container.ID, types.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
		RemoveLinks:   false,
	})
	if err != nil {
		panic(fmt.Errorf("Failed to remove %s\n\t%s", containerID[:6], err.Error()))
	}

	res, err := docker.ContainerCreate(
		context.Background(),
		container.Config,
		container.HostConfig,
		&network.NetworkingConfig{
			EndpointsConfig: container.NetworkSettings.Networks,
		},
		container.Name,
	)
	if err != nil {
		panic(err)
	}

	err = docker.ContainerStart(context.Background(), res.ID, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", res.ID[:len(containerID)])
}
