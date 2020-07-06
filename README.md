# watchtower

![CI](https://github.com/karimsa/watchtower/workflows/CI/badge.svg)

Watch and automatically update docker containers.

Built on top of docker-cli and `moby/moby`, which enables it to automatically support
private registries and the like.

## Tags

**latest** - Latest stable version of watchtower.
**unstable** - Latest untested version of watchtower.

## Installation

Running via docker:

```shell
$ docker run \
	-d \
	--restart=on-failure \
	--name watchtower \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	karimsa/watchtower
```

Running without docker: **not supported yet**

## Usage

In a single update cycle, watchtower does the following:

 1. Check all running containers
 2. Pull the latest image for each running container's configured image.
 3. If the container's image is out-of-date from the system's latest image, rebuilds the container with the same configuration.

As a result, your containers will always be up-to-date with the latest copy of the image.

By default when you run `watchtower` as a CLI or a container, it will run the update cycle exactly once. To keep it running, you need to specify an update interval for how frequently watchtower should check for container updates.

To see the full list of options, simply run watchtower with the `--help` flag.

### Examples

*In these examples, we assume the alias: `alias watchtower="docker run --rm -it -v /var/run/docker.sock:/var/run/docker.sock:ro karimsa/watchtower watchtower"`*

  * **Ensure that the 'jpillora/dnsmasq' containers are up-to-date**: `watchtower --image 'jpillora/dnsmasq'`
  * **Update 'jpillora/dnsmasq' containers every 60 seconds**: `watchtower --image jpillora/dnsmasq --interval 60`

## License

Licensed under [MIT license](LICENSE).

Copyright &copy; 2020-present Karim Alibhai.
