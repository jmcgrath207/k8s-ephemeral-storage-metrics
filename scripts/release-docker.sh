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

if [ -z "$GITHUB_TOKEN" ]; then
  echo "The GITHUB_TOKEN env is not set."
  exit 1
fi


sed -i "s/tag.*/tag: ${VERSION}/g" chart/values.yaml
sed -i "s/version.*/version: ${VERSION}/g" chart/Chart.yaml
sed -i "s/appVersion.*/appVersion: ${VERSION}/g" chart/Chart.yaml

docker login ghcr.io --username jmcgrath207 -p "${GITHUB_TOKEN}"
docker build -f Dockerfile -t ghcr.io/jmcgrath207/$package:$VERSION .
docker build -f Dockerfile -t ghcr.io/jmcgrath207/$package:latest .
docker push ghcr.io/jmcgrath207/$package:$VERSION
docker push ghcr.io/jmcgrath207/$package:latest