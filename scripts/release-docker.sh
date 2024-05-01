#!/bin/bash
#
# Brief description of your script
# Copyright 2023 john

set -e

PACKAGE=k8s-ephemeral-storage-metrics

if [ -z "$VERSION" ]; then
	echo "The VERSION env is not set."
	exit 1
fi

gh auth token | docker login ghcr.io --username jmcgrath207 --password-stdin

if docker buildx ls | grep -q "$PACKAGE"; then
	echo "Instance '$PACKAGE' already exists, skipping creation."
	docker buildx use "$PACKAGE"
else
	docker buildx create --name "$PACKAGE" --use
fi

function build_docker_image() {
  local image=$1
  local dockerfile=$2
  local tag=$3

  docker buildx build \
    --platform linux/arm64/v8,linux/amd64 \
    --tag "ghcr.io/jmcgrath207/${image}:${tag}" \
    --file "${dockerfile}" \
    --push \
    .
}

build_docker_image "${PACKAGE}" "Dockerfile" "$VERSION"
build_docker_image "k8s-ephemeral-storage-grow-test" "DockerfileTestGrow" "$VERSION"
build_docker_image "k8s-ephemeral-storage-shrink-test" "DockerfileTestShrink" "$VERSION"

# Don't push the latest image tags if they are a release candidate
if ! [[ $ENV =~ "rc" ]]; then
	build_docker_image "${PACKAGE}" "Dockerfile" "latest"
  build_docker_image "k8s-ephemeral-storage-grow-test" "DockerfileTestGrow" "latest"
  build_docker_image "k8s-ephemeral-storage-shrink-test" "DockerfileTestShrink" "latest"
fi
