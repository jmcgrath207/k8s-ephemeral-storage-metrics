#!/bin/bash
#
# Brief description of your script
# Copyright 2023 john

set -e

CURRENT_DIR="$(realpath $(dirname ${BASH_SOURCE[0]}))"
cd "${CURRENT_DIR}" || true

source helpers.sh

function main() {
  local image_tag
  local dockerfile
  local registry
  local image
  local common_set_values
  local common_set_values_arr
  local grow_repo_image
  local shrink_repo_image
  local e2e_values_arr

  trap 'trap_func' EXIT ERR

  while [ "$(kubectl get pods -n kube-system -l kubernetes.io/minikube-addons=registry -o=jsonpath='{.items[*].status.phase}')" != "Running Running" ]; do
    echo "waiting for minikube registry and proxy pod to start. Sleep 10" && sleep 10
  done
  # Need both. External to push and internal for pods to pull from registry in cluster
  external_registry="$(minikube ip):5000"
  internal_registry="$(kubectl get service -n kube-system registry --template='{{.spec.clusterIP}}')"

  ### Build and Load Images ###

  grow_repo_image="k8s-ephemeral-storage-grow-test:latest"

  docker build  --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ../DockerfileTestGrow \
  -t "${external_registry}/${grow_repo_image}" -t "${internal_registry}/${grow_repo_image}" ../.

  docker save "${external_registry}/${grow_repo_image}" > /tmp/image.tar
  ${LOCALBIN}/crane push --insecure /tmp/image.tar "${external_registry}/${grow_repo_image}"
  rm /tmp/image.tar


  shrink_repo_image="k8s-ephemeral-storage-shrink-test:latest"

  docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ../DockerfileTestShrink \
  -t "${external_registry}/${shrink_repo_image}" -t "${internal_registry}/${shrink_repo_image}" ../.

  docker save "${external_registry}/${shrink_repo_image}" > /tmp/image.tar
  ${LOCALBIN}/crane push --insecure /tmp/image.tar "${external_registry}/${shrink_repo_image}"
  rm /tmp/image.tar

  if [[ $ENV == "debug" ]]; then
    image_tag="debug-latest"
    dockerfile="DockerfileDebug"
  else
    image_tag="latest"
    dockerfile="Dockerfile"
  fi

  # Main image
  main_repo_image="${DEPLOYMENT_NAME}:${image_tag}"
  docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ../${dockerfile} \
  -t "${external_registry}/${main_repo_image}" -t "${internal_registry}/${main_repo_image}" ../.

  docker save "${external_registry}/${main_repo_image}" > /tmp/image.tar
  ${LOCALBIN}/crane push --insecure /tmp/image.tar "${external_registry}/${main_repo_image}"
  rm /tmp/image.tar


  ### Install Chart ###

  common_set_values_arr=(
    "image.repository=${internal_registry}/$DEPLOYMENT_NAME"
    "image.tag=${image_tag}"
    "deploy_type=Deployment"
    "log_level=debug"
    "dev.enabled=true"
    "dev.shrink.image=${internal_registry}/${shrink_repo_image}"
    "dev.grow.image=${internal_registry}/${grow_repo_image}"
    "metrics.adjusted_polling_rate=true"
    "pprof=true"
  )

  if [[ $ENV =~ "e2e" ]]; then

    e2e_values_arr=("interval=5")
    common_set_values_arr+=("${e2e_values_arr[@]}")
  fi

  common_set_values=$(echo ${common_set_values_arr[*]} | sed 's/, /,/g' | sed 's/ /,/g')

  helm upgrade --install "${DEPLOYMENT_NAME}" ../chart \
    --set "${common_set_values}" \
    --create-namespace \
    --namespace "${DEPLOYMENT_NAME}"


  # Patch deploy so minikube image upload works.
  if [[ $ENV == "debug" ]]; then
    # Disable for Debugging of Delve.
    kubectl patch deployments.apps -n "${DEPLOYMENT_NAME}" k8s-ephemeral-storage-metrics -p \
      '{ "spec": {"template": { "spec":{"securityContext": null, "containers":[{"name":"metrics", "livenessProbe": null, "readinessProbe": null, "securityContext": null, "command": null, "args": null  }]}}}}'
  fi

  # Kill dangling port forwards if found.
  # Main Exporter Port
  sudo ss -aK '( dport = :9100 or sport = :9100 )' | true
  # Prometheus Port
  sudo ss -aK '( dport = :9090 or sport = :9090 )' | true
  # Pprof Port
  sudo ss -aK '( dport = :6060 or sport = :6060 )' | true

  # Start Exporter Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/k8s-ephemeral-storage-metrics 9100:9100 || sleep 5; done
  ) &

  # Wait until main pod comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "waiting for k8s-ephemeral-storage-metrics  pod to start. Sleep 10" && sleep 10
  done

  if [[ $ENV == "debug" ]]; then
    # Background log following for manager
    (
      sleep 10
      printf "\n\n" && while :; do kubectl logs -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -f || sleep 5; done
    ) &

    kubectl port-forward -n $DEPLOYMENT_NAME services/debug 30002:30002

  elif [[ $ENV == "e2e" ]]; then
    ${LOCALBIN}/ginkgo -v -r ../tests/e2e/...
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
