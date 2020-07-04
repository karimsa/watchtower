package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	networkTypes "github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
)

const (
	eraseEndLine = "\u001B[K"
)

var (
	showHelp    = flag.Bool("help", false, "Print this help message")
	imageIDs    = flag.String("imageIDs", "", "RegExp to match image IDs")
	interval    = flag.Duration("interval", 1*time.Minute, "Duration in between checks")
	watch       = flag.Bool("watch", false, "Keep watchtower alive (if false, only checks once)")
	exitOnError = flag.Bool("bail", false, "Exit as soon as an error occurs")
)

func usage() {
	fmt.Fprintf(flag.NewFlagSet(os.Args[0], flag.ExitOnError).Output(), "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

type watchtower struct {
	docker       *docker.Client
	registryAuth map[string]string
}

func (wt *watchtower) loadDockerConfig() error {
	data, err := ioutil.ReadFile(os.Getenv("HOME") + "/.docker/config.json")
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			wt.registryAuth = make(map[string]string)
			return nil
		}
		return err
	}

	var config struct {
		Auths map[string]struct {
			Auth string
		}
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	wt.registryAuth = make(map[string]string, len(config.Auths))
	for registry, opts := range config.Auths {
		wt.registryAuth[registry] = opts.Auth
	}
	return nil
}

func (wt *watchtower) pullImage(auth, image string) error {
	log.Printf("Checking image: %s\n", image)
	reader, err := wt.docker.ImagePull(context.Background(), image, dockerTypes.ImagePullOptions{
		All:          true,
		RegistryAuth: auth,
		PrivilegeFunc: func() (string, error) {
			return auth, nil
		},
	})
	if err != nil {
		log.Printf("Failed to fetch %s: %s\n", image, err.Error())
		return err
	}

	bufReader := bufio.NewReader(reader)
	for {
		line, _, err := bufReader.ReadLine()
		if err == io.EOF {
			fmt.Printf("\r%s", eraseEndLine)
			return nil
		}
		if err != nil {
			return err
		}

		data := make(map[string]interface{})
		if err := json.Unmarshal(line, &data); err != nil {
			return err
		}
		if err, ok := data["error"]; ok {
			return errors.New(err.(string))
		}
		if progress, ok := data["progress"]; ok {
			fmt.Printf("%s %s%s\r", image, progress.(string), eraseEndLine)
		} else if !strings.Contains(data["status"].(string), "uses outdated schema1 manifest format") {
			fmt.Printf("%s: %s%s\r", image, data["status"].(string), eraseEndLine)
		}
	}
}

func (wt *watchtower) getContainerImageName(name string, container dockerTypes.Container) (string, string, error) {
	image := container.Image
	registry := "hub.docker.com"

	if strings.HasPrefix(image, "sha256:") {
		// func (cli *Client) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
		imageDetails, _, err := wt.docker.ImageInspectWithRaw(context.Background(), image)
		if err != nil {
			return image, registry, fmt.Errorf("Failed to inspect image %s\n\t%s", image[:7+6], err.Error())
		}

		for _, tag := range imageDetails.RepoTags {
			imageEnd := strings.Index(tag, ":")
			if imageEnd > -1 {
				image = tag[:imageEnd]
			} else {
				image = tag
			}
		}
	}
	if strings.HasPrefix(image, "sha256:") {
		return image, registry, fmt.Errorf("Failed to resolve image for container: %s", name)
	}

	switch strings.Count(image, "/") {
	case 0:
		image = "docker.io/library/" + image
	case 1:
		image = "docker.io/" + image
	case 2:
		registry = image[:strings.Index(image, "/")]
	default:
		return image, registry, fmt.Errorf("Unexpected number of slashes in: %s\n", image)
	}

	return image, registry, nil
}

func (wt *watchtower) rebuildContainer(containerID string) error {
	container, err := wt.docker.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return err
	}

	// TODO: Should store the container's config somewhere, in case of failure between the remove and the
	// start
	err = wt.docker.ContainerRemove(context.Background(), container.ID, dockerTypes.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
		RemoveLinks:   false,
	})
	if err != nil {
		return fmt.Errorf("Failed to remove %s\n\t%s", containerID[:6], err.Error())
	}

	res, err := wt.docker.ContainerCreate(
		context.Background(),
		container.Config,
		container.HostConfig,
		&networkTypes.NetworkingConfig{
			EndpointsConfig: container.NetworkSettings.Networks,
		},
		container.Name,
	)
	if err != nil {
		return err
	}

	err = wt.docker.ContainerStart(context.Background(), res.ID, dockerTypes.ContainerStartOptions{})
	if err != nil {
		return err
	}

	log.Printf("Restarted %s container: %s as %s\n", container.Image, containerID[:6], res.ID[:6])
	return nil
}

func (wt *watchtower) checkUpdates() {
	log.Printf("Checking for updates ...")

	containers, err := wt.docker.ContainerList(context.Background(), dockerTypes.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	imagesUpdated := make(map[string]bool, len(containers))

	for _, container := range containers {
		_, withCompose := container.Labels["com.docker.compose.config-hash"]

		if !withCompose && container.State == "running" {
			name := container.ID[:6]
			if len(container.Names) > 0 {
				name = container.Names[0]
			}

			image, registry, err := wt.getContainerImageName(name, container)
			if err != nil {
				imagesUpdated[image] = false
				log.Printf(err.Error())
				if *exitOnError {
					os.Exit(1)
				}
				continue
			}

			wasUpdated, imageChecked := imagesUpdated[image]
			auth, _ := wt.registryAuth[registry]

			if imageChecked {
				if wasUpdated {
					log.Printf("Restarting container: %s (%s)\n", container.ID, image)
				}
			} else {
				if err := wt.pullImage(auth, image); err != nil {
					imagesUpdated[image] = false
					log.Printf("Failed to fetch %s\n%s\n", image, err.Error())
					if *exitOnError {
						os.Exit(1)
					}
					continue
				}

				imageDetails, _, err := wt.docker.ImageInspectWithRaw(context.Background(), image)
				if err != nil {
					imagesUpdated[image] = false
					log.Printf("Failed to inspect %s\n%s\n", image, err.Error())
					if *exitOnError {
						os.Exit(1)
					}
					continue
				}

				if imageDetails.ID != container.ImageID {
					imagesUpdated[image] = true

					if err := wt.rebuildContainer(container.ID); err != nil {
						log.Printf("Failed to update %s\n\t%s\n", name, err.Error())
						if *exitOnError {
							os.Exit(1)
						}
						continue
					}
				} else {
					imagesUpdated[image] = false
					log.Printf("%s is up-to-date\n", name)
				}
			}
		}
	}
}

func main() {
	flag.Parse()

	if *showHelp {
		usage()
		return
	}

	cli, err := docker.NewEnvClient()
	if err != nil {
		panic(err)
	}

	wt := &watchtower{
		docker: cli,
	}
	if err := wt.loadDockerConfig(); err != nil {
		panic(err)
	}

	_ = imageIDs

	wt.checkUpdates()
	for *watch {
		fmt.Printf("------------------------\n")
		for i := 0 * time.Minute; i < *interval; i += 1 * time.Second {
			fmt.Printf("\r%sNext update check in %s", eraseEndLine, *interval-i)
			time.Sleep(1 * time.Second)
		}
		fmt.Printf("\r%s", eraseEndLine)

		wt.checkUpdates()
	}
}
