#!/bin/bash
set -e
set -o pipefail

export PATH="$PATH:`dirname $0`"

function log() {
	echo "[`date`] $@"
}

function show_usage() {
	echo -e ""
	echo -e "usage: watchtower [OPTIONS]"
	echo -e ""
	echo -e "Options:"
	echo -e "	-h, --help	print this help message"
	echo -e "	-i, --interval [number]	continuously runs update cycles, with [number] seconds in between"
	echo -e "	-I, --image [pattern]	filters checked images to ones matching [pattern]"
	echo -e "	-p, --prune	prune images after successful updates"
	echo -e ""
	exit 1
}

function erase_end_line() {
	if which tput &>/dev/null; then
		tput el
	else
		# Max width of lines should be 103, given that we trim
		# long lines at 100 and then add an ellipsis
		echo -en "\r"
		for ((x=0;x<104;x++)); do
			echo -en " "
		done
		echo -en "\r"
	fi
}

if test -e "`dirname $0`/rebuild_container"; then
	alias rebuild_container="`dirname $0`/rebuild_container"
fi

interval=""
imageFilter=".*"
pruneImages="false"
while test "$#" -gt "0"; do
	case "$1" in
		-h|--help)
			show_usage
			;;

		-i|--interval)
			shift
			interval="$1"
			shift || (echo "Missing interval value for --interval"; show_usage)
			if ! test -z "$interval" && test "$[0+$interval]" = "0"; then
				echo "Invalid integer value passed for --interval"
				show_usage
			fi
			;;

		-I|--image)
			shift
			imageFilter="$1"
			shift || (echo "Missing pattern for --image"; show_usage)

			if (echo "" | grep -E "$imageFilter" >/dev/null; test "$?" = "2"); then
				echo "Error: Invalid regex passed for --image"
				show_usage
			fi
			;;

		-p|--prune)
			shift
			pruneImages="true"
			;;

		-*)
			echo "Unknown flag: $1"
			show_usage
			;;

		*)
			echo "Unknown argument: $1"
			show_usage
			;;
	esac
done

if ! test -e "$HOME/.docker/config.json"; then
	log "Warning: No docker configuration found at: ~/.docker/config.json" >&2
	log "Warning: Registry authentication might not work." >&2
fi

cat ~/.docker/config.json | jq -r '.auths | keys | .[]' | while read url; do
	log "Found configured auth for: $url"
done

while :; do
	visitedImages="|"
	numUpdatedImages="0"
	numUpdatedContainers="0"

	for containerID in `docker ps -q`; do
		containerImageID=`docker inspect -f '{{.Image}}' $containerID`
		containerImageID=${containerImageID#*:}

		imageName=`docker inspect -f '{{.Config.Image}}' $containerID`
		if ! echo "${imageName}" | grep ":" &>/dev/null; then
			imageName="${imageName}:latest"
		fi

		if echo "$imageName" | grep -E "$imageFilter" &>/dev/null; then
			if echo "$visitedImages" | tr '|' '\n' | grep "$imageName"; then
				:
			else
				log "Pulling: $imageName"
				preUpdateImage=`docker images --format '{{.ID}}' $imageName`
				docker pull $imageName | tee test.log | while read stdout; do
					echo -en "\r"
					erase_end_line

					output="$imageName - $stdout"
					echo -en "${output:0:100}"
					if ! test -z "${output:100}"; then
						echo -en "..."
					fi
				done

				echo -en "\r"
				erase_end_line

				imageID=`docker images --format '{{.ID}}' $imageName`
				if test "$imageID" = "$preUpdateImage"; then
					log "Image up-to-date: $imageName"
				else
					log "Updated image to $imageID: $imageName"
					numUpdatedImages="$[1+$numUpdatedImages]"
				fi

				visitedImages="${visitedImages}${imageName}|"
			fi

			imageID=`docker images --format '{{.ID}}' $imageName`
			if test "${containerImageID:0:${#imageID}}" != "$imageID"; then
				log "Container out-of-date: $containerID (running ${containerImageID:0:${#imageID}}, expected $imageID)"

				if echo "$imageName" | grep "karimsa/watchtower" &>/dev/null; then
					log "Restarted $containerID as `rebuild_container --allow-dup "$containerID"`"
				else
					log "Restarted $containerID as `rebuild_container "$containerID"`"
				fi

				numUpdatedContainers="$[1+$numUpdatedContainers]"
			fi
		fi
	done

	log "Updated $numUpdatedContainers containers & $numUpdatedImages images."

	if test "$numUpdatedContainers" -gt "0" && test "$pruneImages" = "true"; then
		docker images prune -a
	fi

	if test -z "$interval"; then
		exit
	fi

	echo "---------------------------"
	for ((i=0;i<$interval;i++)); do
		echo -en "\r"
		erase_end_line
		echo -en "Next update in $[$interval-$i]s"
		sleep 1
	done
	echo -en "\r"
	erase_end_line
done
