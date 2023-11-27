#!/bin/bash
#
# Brief description of your script
# Copyright 2023 john

set -e

source scripts/helpers.sh


function main() {
  local image_tag
  local dockerfile

  trap 'trap_func' EXIT ERR

  if [[ $ENV == "debug" ]]; then
    image_tag="debug-latest"
    dockerfile="DockerfileDebug"
  else
    image_tag="latest"
    dockerfile="Dockerfile"
  fi

  common_set_values_arr=(
  "image.repository=local.io/local/$DEPLOYMENT_NAME",
  "image.tag=${image_tag}",
  "image.imagePullPolicy=Never",
  "deploy_type=Deployment",
  "log_level=debug"
  "dev.enabled=true",
  "metrics.adjusted_polling_rate=true"
  )

  if [[ $ENV =~ "e2e"   ]]; then
    docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f DockerfileTestGrow -t local.io/local/grow-test:latest .
    kind load docker-image -v 9 --name "${DEPLOYMENT_NAME}-cluster" --nodes "${DEPLOYMENT_NAME}-cluster-worker" "local.io/local/grow-test:latest"

    docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f DockerfileTestShrink -t local.io/local/shrink-test:latest .
    kind load docker-image -v 9 --name "${DEPLOYMENT_NAME}-cluster" --nodes "${DEPLOYMENT_NAME}-cluster-worker" "local.io/local/shrink-test:latest"

    e2e_values_arr=("interval=5")
    common_set_values_arr+=("${e2e_values_arr[@]}")
  fi

  common_set_values=$(echo ${common_set_values_arr[*]} | sed 's/, /,/g' | sed 's/ /,/g')

  docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ${dockerfile} -t local.io/local/$DEPLOYMENT_NAME:$image_tag .
  kind load docker-image -v 9 --name "${DEPLOYMENT_NAME}-cluster" --nodes "${DEPLOYMENT_NAME}-cluster-worker" "local.io/local/${DEPLOYMENT_NAME}:${image_tag}"



  # Install Par Chart
  helm upgrade --install $DEPLOYMENT_NAME ./chart \
    --set "${common_set_values}" \
    --create-namespace \
    --namespace $DEPLOYMENT_NAME

  # Patch deploy so Kind image upload to work.
  if [[ $ENV == "debug" ]]; then
    # Disable for Debugging of Delve.
    kubectl patch deployments.apps -n $DEPLOYMENT_NAME k8s-ephemeral-storage-metrics -p \
      '{ "spec": {"template": { "spec":{"securityContext": null, "containers":[{"name":"metrics", "livenessProbe": null, "readinessProbe": null, "securityContext": null, "command": null, "args": null  }]}}}}'
  fi


  # kill dangling port forwards if found.
  # Exporter Porter
  sudo ss -aK '( dport = :9100 or sport = :9100 )' | true
  # Prometheus Port
  sudo ss -aK '( dport = :9090 or sport = :9090 )' | true

  # Start Exporter Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/k8s-ephemeral-storage-metrics 9100:9100 || sleep 5; done
  ) &

  # Start Prometheus Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/prometheus-operated  9090:9090 || sleep 5; done
  ) &

  if [[ $ENV == "debug" ]]; then
    # Background log following for manager
    (
      sleep 10
      printf "\n\n" && while :; do kubectl logs -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics  -f || sleep 5; done
    ) &

    while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
        echo  "waiting for k8s-ephemeral-storage-metrics  pod to start. Sleep 10" && sleep 10
    done
    kubectl port-forward -n $DEPLOYMENT_NAME services/debug 30002:30002

  elif [[ $ENV == "e2e" ]]; then
    ${LOCALBIN}/ginkgo -v -r ./tests/e2e/...
  elif [[ $ENV == "e2e-debug" ]]; then
    sleep infinity
  else
    # Assume make local deploy
    # Background log following for manager
    (
      sleep 10
      printf "\n\n" && while :; do kubectl logs -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -f || sleep 5; done
    ) &
    sleep infinity
  fi
}

main "$@"
