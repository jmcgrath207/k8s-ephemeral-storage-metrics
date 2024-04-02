#!/bin/bash
#
# Brief description of your script
# Copyright 2023 john

set -e


package=k8s-ephemeral-storage-metrics

if [ -z "$VERSION" ]; then
  echo "The VERSION env is not set."
  exit 1
fi


gh auth token | docker login ghcr.io --username jmcgrath207 --password-stdin
docker build -f Dockerfile -t ghcr.io/jmcgrath207/$package:$VERSION .
docker build -f Dockerfile -t ghcr.io/jmcgrath207/$package:latest .
docker push ghcr.io/jmcgrath207/$package:$VERSION

docker build -f DockerfileTestGrow -t ghcr.io/jmcgrath207/k8s-ephemeral-storage-grow-test:latest .
docker build -f DockerfileTestGrow -t ghcr.io/jmcgrath207/k8s-ephemeral-storage-grow-test:$VERSION .
docker push ghcr.io/jmcgrath207/k8s-ephemeral-storage-grow-test:$VERSION

docker build -f DockerfileTestGrow -t ghcr.io/jmcgrath207/k8s-ephemeral-storage-shrink-test:latest .
docker build -f DockerfileTestGrow -t ghcr.io/jmcgrath207/k8s-ephemeral-storage-shrink-test:$VERSION .
docker push ghcr.io/jmcgrath207/k8s-ephemeral-storage-shrink-test:$VERSION

# Don't push the latest image tags if they are a release candidate
if ! [[ $ENV =~ "rc" ]]; then
  docker push ghcr.io/jmcgrath207/$package:latest
  docker push ghcr.io/jmcgrath207/k8s-ephemeral-storage-shrink-test:latest
  docker push ghcr.io/jmcgrath207/k8s-ephemeral-storage-grow-test:latest
fi