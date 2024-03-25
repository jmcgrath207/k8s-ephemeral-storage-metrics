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
  local common_set_values
  local common_set_values_arr
  local grow_repo_image
  local shrink_repo_image
  local e2e_values_arr
  local external_registry
  local internal_registry
  local status_code

  trap 'trap_func' EXIT ERR

  # Wait until registry pod come up
  while [ "$(kubectl get pods -n kube-system -l actual-registry=true -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for registry pod to start. Sleep 10" && sleep 10
  done

  # Wait until registry-proxy pod come up
  while [ "$(kubectl get pods -n kube-system -l registry-proxy=true -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for registry proxy pod to start. Sleep 10" && sleep 10
  done

  # Use a while loop to repeatedly check the registry endpoint until health
  while true; do
   # Send a GET request to the endpoint and capture the HTTP status code
   status_code=$(curl -s -o /dev/null -w "%{http_code}" "http://$(minikube ip):5000/v2/_catalog")

   # Check if the status code is 200
   if [ "$status_code" -eq 200 ]; then
      echo "Registry endpoint is healthy. Status code: $status_code"
      break
   else
      echo "Registry endpoint is not healthy. Status code: $status_code. Retrying..."
      sleep 5 # Wait for 5 seconds before retrying
   fi
  done

  # Need both. External to push and internal for pods to pull from registry in cluster
  external_registry="$(minikube ip):5000"
  internal_registry="$(kubectl get service -n kube-system registry --template='{{.spec.clusterIP}}')"

  ### Build and Load Images ###

  grow_repo_image="k8s-ephemeral-storage-grow-test:latest"

  docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ../DockerfileTestGrow \
    -t "${external_registry}/${grow_repo_image}" -t "${internal_registry}/${grow_repo_image}" ../.

  docker save "${external_registry}/${grow_repo_image}" >/tmp/image.tar
  "${LOCALBIN}/crane" push --insecure /tmp/image.tar "${external_registry}/${grow_repo_image}"
  rm /tmp/image.tar

  shrink_repo_image="k8s-ephemeral-storage-shrink-test:latest"

  docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f ../DockerfileTestShrink \
    -t "${external_registry}/${shrink_repo_image}" -t "${internal_registry}/${shrink_repo_image}" ../.

  docker save "${external_registry}/${shrink_repo_image}" >/tmp/image.tar
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

  docker save "${external_registry}/${main_repo_image}" >/tmp/image.tar
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

  # Start Exporter Port Forward
  (
    sleep 10
    printf "\n\n" && while :; do kubectl port-forward -n $DEPLOYMENT_NAME service/k8s-ephemeral-storage-metrics 9100:9100 || kill_main_exporter_port && sleep 5; done
  ) &

  # Wait until main pod comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l app.kubernetes.io/name=k8s-ephemeral-storage-metrics -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for k8s-ephemeral-storage-metrics pod to start. Sleep 10" && sleep 10
  done

  # Wait until grow-test comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l name=grow-test -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for grow-test pod to start. Sleep 10" && sleep 10
  done

  # Wait until shrink-test comes up
  while [ "$(kubectl get pods -n $DEPLOYMENT_NAME -l name=shrink-test -o=jsonpath='{.items[*].status.phase}')" != "Running" ]; do
    echo "Waiting for shrink-test pod to start. Sleep 10" && sleep 10
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
